package main

import (
	"fmt"
	"log"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/irsl/broker-ari/arimsgs"
)

func parseRawMessage(rawMsg []byte) (*arimsgs.ParametersMsg, error) {
	pm := &arimsgs.ParametersMsg{}
	if err := proto.Unmarshal(rawMsg, pm); err != nil {
		return nil, err
	}
	return pm, nil
}

func parseParameterMessageToMap(pmsg *arimsgs.ParametersMsg) map[string]any {
	re := map[string]any{}
	for _, p := range pmsg.Params {
		var v any = p.ValueS
		if p.ValueS == "" {
			v = p.ValueI
		}
		re[p.Key] = v
	}
	log.Printf("proto decoded as: %+v", re)
	return re
}

func parseRawMessageToMap(rawMsg []byte) (map[string]any, *arimsgs.ParameterLimitsMsg, error) {
	p, err := parseRawMessage(rawMsg)
	if err != nil {
		return nil, nil, err
	}
	return parseParameterMessageToMap(p), p.ParamLimitsMsg, nil
}

func getParamMessage(cats []string) *arimsgs.ParametersMsg {
	p := &arimsgs.ParametersMsg{}
	p.Timestamp = time.Now().UnixNano()
	for i, c := range cats {
		pm := &arimsgs.Parameter{
			Key:        fmt.Sprintf("P%d", i+1),
			Something1: 5,
			ValueS:     c,
		}
		p.Params = append(p.Params, pm)
	}

	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "requester.client.id",
		Something1: 5,
		ValueS:     "inline",
	})
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "request.id",
		Something1: 5,
		ValueS:     "params",
	})

	return p
}

func getParamMessageRaw(cats []string) ([]byte, error) {
	p := getParamMessage(cats)
	return proto.Marshal(p)
}

func putParams(cat string, value int32) ([]byte, error) {
	p := &arimsgs.ParametersMsg{}
	p.Timestamp = time.Now().UnixNano()
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        cat,
		Something1: 3,
		ValueI:     value,
	})

	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "requester.client.id",
		Something1: 5,
		ValueS:     "inline",
	})
	p.Params = append(p.Params, &arimsgs.Parameter{
		Key:        "request.id",
		Something1: 5,
		ValueS:     "result",
	})

	return proto.Marshal(p)
}
