package main

import (
	"fmt"
	"log"
	"time"

	"github.com/irsl/broker-ari/arimsgs"
	"google.golang.org/protobuf/proto"
)

func parser_log_Printf(t string, params ...any) {
	if !Config.Parser_debug {
		return
	}
	log.Printf(t, params...)
}


func parseParams(msg *arimsgs.ParametersMsg) (map[string]int32, map[string]arimsgs.ParameterLimit) {
	var paramResult = map[string]int32{}
	var limitResult = map[string]arimsgs.ParameterLimit{}

	for _, b := range msg.Params {
		paramResult[b.Key] = b.GetValueI()
	}
	for _, c := range msg.ParamLimitsMsg.ParamLimits {
		var minMaxResult arimsgs.ParameterLimit
		minMaxResult.Max = c.Max
		minMaxResult.Min = c.Min
		limitResult[c.Key] = minMaxResult
	}
	return paramResult, limitResult
}

func parseBirthMessage(msg *arimsgs.ParametersMsg) map[string]string {
	var birthResult = map[string]string{}
	for _, b := range msg.Params {
		birthResult[b.Key] = b.GetValueS()
	}
	return birthResult
}

func parseRawMessage(rawMsg []byte) (*arimsgs.ParametersMsg, error) {
	pm := &arimsgs.ParametersMsg{}
	if err := proto.Unmarshal(rawMsg, pm); err != nil {
		return nil, err
	}
	parser_log_Printf("%s", pm)
	return pm, nil
}

func parseConsumptionMessage(rawMsg []byte) (*arimsgs.ConsumptionMsg, error) {
	cm := &arimsgs.ConsumptionMsg{}
	if err := proto.Unmarshal(rawMsg, cm); err != nil {
		return nil, err
	}
	parser_log_Printf("%s", cm)
	return cm, nil
}

func getParamMessage(cats []string) *arimsgs.ParametersMsg {
	p := &arimsgs.ParametersMsg{}
	p.Timestamp = time.Now().UnixNano()
	for i, c := range cats {
		pm := &arimsgs.Parameter{
			Key:        fmt.Sprintf("P%d", i+1),
			Something1: 5,
			Value:      &arimsgs.Parameter_ValueS{ValueS: c},
		}
		p.Params = append(p.Params, pm)
	}

	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "requester.client.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "inline"},
	})
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "request.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "params"},
	})

	return p
}

func getConsumptionParamMessage(typ string) *arimsgs.ParametersMsg {
	p := &arimsgs.ParametersMsg{}
	p.Timestamp = time.Now().UnixNano()
	pm := &arimsgs.Parameter{
		Key:        "Typ",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: typ},
	}
	p.Params = append(p.Params, pm)

	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "requester.client.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "inline"},
	})
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "request.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "consumptions"},
	})

	return p
}

func getParamMessageRaw(cats []string) ([]byte, error) {
	p := getParamMessage(cats)
	return proto.Marshal(p)
}

func getConsumptionParamMessageRaw(typ string) ([]byte, error) {
	p := getConsumptionParamMessage(typ)
	return proto.Marshal(p)
}

func putParams(cat string, value int32) ([]byte, error) {
	p := &arimsgs.ParametersMsg{}
	p.Timestamp = time.Now().UnixNano()
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        cat,
		Something1: 3,
		Value:      &arimsgs.Parameter_ValueI{ValueI: value},
	})

	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "requester.client.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "inline"},
	})
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "request.id",
		Something1: 5,
		Value:      &arimsgs.Parameter_ValueS{ValueS: "result"},
	})

	return proto.Marshal(p)
}
