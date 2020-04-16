package main

import (
	"encoding/json"
	"flag"
	"log"
	_ "net/http"
	_ "net/http/pprof"

	"github.com/m-pavel/go-tion/impl"

	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/m-pavel/go-hassio-mqtt/pkg"

	"net/http"

	"github.com/m-pavel/go-tion/tion"
)

const timeout = 7 * time.Second

// TionService instance
type TionService struct {
	t     tion.Tion
	bt    *string
	debug bool

	keepbt *bool
	ctx    *ghm.ServiceContext
}

// PrepareCommandLineParams for TionService
func (ts *TionService) PrepareCommandLineParams() {
	ts.bt = flag.String("device", "xx:yy:zz:aa:bb:cc", "Device BT address")
	ts.keepbt = flag.Bool("keepbt", false, "Keep bluetooth connection")

}

// Name of TionService
func (ts TionService) Name() string { return "tion" }

func (ts *TionService) OnConnect(client MQTT.Client, topic, topicc, topica string) {
	if token := client.Subscribe(topicc, 2, ts.control); token.WaitTimeout(timeout) && token.Error() != nil {
		log.Println(token.Error())
	}
}

// Init TionService
func (ts *TionService) Init(ctx *ghm.ServiceContext) error {
	go func() {
		log.Println(http.ListenAndServe(":7070", nil))
	}()
	ts.t = impl.NewTionImpl(*ts.bt, ctx.Debug(), nil)
	ts.debug = ctx.Debug()
	ts.ctx = ctx
	if *ts.keepbt {
		return ts.t.Connect(timeout)
	}
	return nil
}
func (ts *TionService) cmStart() error {
	if *ts.keepbt && ts.t.Connected() {
		return nil
	}
	return ts.t.Connect(timeout)
}
func (ts *TionService) cmEnd() error {
	if *ts.keepbt {
		return nil
	}
	return ts.t.Disconnect(timeout)
}

// Do TionService
func (ts TionService) Do() (interface{}, error) {
	if err := ts.cmStart(); err != nil {
		return nil, err
	}
	defer func() {
		if err := ts.cmEnd(); err != nil {
			log.Println(err)
		}
	}()
	s, err := ts.t.ReadState(timeout)
	if err != nil {
		return nil, err
	}

	if ts.debug {
		log.Println(s.BeautyString())
	}
	return tion.RestFromStatus(s), nil
}

func (ts *TionService) control(cli MQTT.Client, msg MQTT.Message) {
	req := tion.RestStatus{}
	err := json.Unmarshal(msg.Payload(), &req)
	if err != nil {
		log.Println(err)
		return
	}
	if ts.debug {
		log.Println(req)
	}

	if err := ts.cmStart(); err != nil {
		log.Println(err)
		return
	}
	defer func() {
		if err := ts.cmEnd(); err != nil {
			log.Println(err)
		}
	}()

	cs, err := ts.t.ReadState(timeout)
	if err != nil {
		log.Println(err)
		return
	}

	needupdate := false
	// 1. Speed
	if req.Speed != nil && *req.Speed != cs.Speed {
		cs.Speed = *req.Speed
		needupdate = true
	}
	// 2. Heater
	if req.Heater != nil {
		if cs.HeaterEnabled != *req.Heater {
			cs.HeaterEnabled = *req.Heater
			needupdate = true
		}
	}
	// 3. Temp
	if req.Target != nil {
		if cs.TempTarget != *req.Target {
			cs.TempTarget = *req.Target
			needupdate = true
		}
	}
	// 4. Gate
	if req.Gate != nil {
		if cs.GateStatus() != *req.Gate {
			cs.SetGateStatus(*req.Gate)
			needupdate = true
		}
	}
	// 5. Sound
	if req.Sound != nil {
		if cs.SoundEnabled != *req.Sound {
			cs.SoundEnabled = *req.Sound
			needupdate = true
		}
	}
	// 77. State
	if req.On != nil {
		if cs.Enabled {
			if !*req.On {
				cs.Enabled = false
				needupdate = true
			} else {
				log.Println("Already on")
			}
		} else {
			if *req.On {
				cs.Enabled = true
				needupdate = true
			} else {
				log.Println("Already off")
			}
		}
	}

	if needupdate {
		if err = ts.t.Update(cs, timeout); err != nil {
			log.Println(err)
		} else {
			if err := ts.ctx.SendState(); err != nil {
				log.Println(err)
			} else {
				log.Println("Made update by MQTT request")
			}
		}
	}

	log.Println("Control done.")
}

// Close TionService
func (ts TionService) Close() error {
	return ts.t.Disconnect(timeout)
}

func main() {
	ghm.NewStub(&TionService{}).Main()
}