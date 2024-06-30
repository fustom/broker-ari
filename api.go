package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	URL "net/url"
	"strings"
)

var (
	apiToken string
)

func commonHandler(handler func(path string, body any, params URL.Values, method string) any) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		if Config.Api_debug {
			req.URL.Scheme = "http"
			req.URL.Host = "localhost"
			bytes, _ := httputil.DumpRequestOut(req, true)
			log.Printf("API request:\n%s", bytes)
		}
		if req.URL.Path != "/accounts/login" && req.Header.Get("Ar.authtoken") != apiToken {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}
		var p any
		if req.Method != "GET" {
			err := json.NewDecoder(req.Body).Decode(&p)
			if err != nil {
				log.Printf("Error while decoding body: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		result := handler(req.URL.Path, p, req.URL.Query(), req.Method)
		json.NewEncoder(w).Encode(result)
		if Config.Api_debug {
			log.Printf("API result: %+v", result)
		}
	}
}

func velisPlants(path string, body any, params URL.Values, method string) any {
	re := []any{}
	for clId, c := range clientMap {
		a := map[string]any{}
		a["gw"] = clId
		a["sn"] = c.birth["serial_number"]
		a["fwVer"] = c.birth["firmware_version"]

		for _, device := range Config.Devices {
			if device.GwID == clId {
				a["sys"] = device.Sys
				a["wheType"] = device.WheType
				a["wheModelType"] = device.WheModelType
				a["name"] = device.Name
			}
		}

		re = append(re, a)
	}
	return re
}

func remotePlants(path string, body any, params URL.Values, method string) any {
	return []any{}
}

func defaultHandler(path string, body any, params URL.Values, method string) any {
	log.Printf("no route for path %s", path)
	return map[string]any{}
}

func login(path string, body any, params URL.Values, method string) any {
	bmap, ok := body.(map[string]any)
	if !ok {
		return nil
	}
	if Config.Api_username != "" {
		err := map[string]string{"error": "invalid username/password"}
		usr := bmap["usr"].(string)
		if usr != Config.Api_username {
			return err
		}
		pwd := bmap["pwd"].(string)
		if pwd != Config.Api_password {
			return err
		}
	}

	return map[string]any{"token": apiToken}
}

func velisPlantDataSet(clID, cat string, value int32) any {
	b, err := putParams(cat, value)
	if err == nil {
		err = server.Publish("$EDC/ari/"+clID+"/ar1/PUT/Menu/Par", b, false, 0)
	}
	if err != nil {
		log.Printf("error for %v %v %v: %v", clID, cat, value, err)
		return nil
	}
	// remembering the new setting so it is returned correctly even if the API is called before the next polling happens
	if c, ok := clientMap[clID]; ok && c.params != nil {
			c.params[cat] = value
	}
	return map[string]bool{"success": true}

}

func busErrors(path string, body any, params URL.Values, method string) any {
	if c, ok := clientMap[params.Get("gatewayId")]; ok && c.errors != nil {
		return map[string]any{} // TODO: return errors
	}
	return map[string]any{}
}

func features(path string, body any, params URL.Values, method string) any {
	v := strings.Split(path, "/")
	clID := v[3]
	if c, ok := clientMap[clID]; ok && c.errors != nil {
		return map[string]any{"hasMetering": true}
	}
	return map[string]any{}
}

func consumption(path string, body any, params URL.Values, method string) any {
	v := strings.Split(path, "/")
	clID := v[3]

	offset := 0
	for _, device := range Config.Devices {
		if device.GwID == clID {
			offset = device.ConsumptionOffset
		}
	}

	if c, ok := clientMap[clID]; ok && c.cWh != nil {
		ret := []map[string]any{}
		for _, consumptions := range c.cWh.Consumptions.Consumptions {
			kwhs := make([]float32, len(consumptions.Wh))
			for i, wh := range consumptions.Wh {
				kwhs[i] = float32(wh) / 1000
			}

			re := map[string]any{}
			re["k"] = consumptions.ConsumptionType + int32(offset)
			re["p"] = consumptions.ConsumptionTimeInterval
			re["v"] = kwhs

			ret = append(ret, re)
		}
		return ret
	}
	return map[string]any{}
}

func medPlantData(path string, body any, params URL.Values, method string) any {
	v := strings.Split(path, "/")
	clID := v[3]
	switch {
	case len(v) == 4:
		re := map[string]any{}
		if c, ok := clientMap[clID]; ok && c.params != nil {
			re["on"] = c.params["T_18.0.0"]
			re["mode"] = c.params["T_18.0.1"]
			re["eco"] = c.params["T_18.0.2"]
			re["pwrOpt"] = c.params["T_18.0.3"]

			re["reqTemp"] = c.params["T_18.1.0"] / 10

			re["antiLeg"] = c.params["T_18.3.0"]
			re["avShw"] = c.params["T_18.3.1"]
			var hours int32 = 0
			var minutes int32 = c.params["T_18.3.2"]
			if minutes > 60 {
				hours = minutes / 60
				minutes -= hours * 60
			}
			re["rmTm"] = fmt.Sprintf("%d:%d:0", hours, minutes)
			re["temp"] = float32(c.params["T_18.3.3"]) / 10
			re["heatReq"] = c.params["T_18.3.5"]
			re["procReqTemp"] = c.params["T_18.3.6"] / 10

			re["gw"] = clID
		}
		return re
	case len(v) == 5 && v[4] == "temperature":
		bodyMap := body.(map[string]any)
		newTemp := int32(bodyMap["new"].(float64)) * 10
		return velisPlantDataSet(clID, "T_18.1.0", newTemp)
	case len(v) == 5 && v[4] == "mode":
		bodyMap := body.(map[string]any)
		newMode := int32(bodyMap["new"].(float64))
		return velisPlantDataSet(clID, "T_18.0.1", newMode)
	case len(v) == 5 && v[4] == "switch":
		newMode := 0
		if body.(bool) {
			newMode = 1
		}
		return velisPlantDataSet(clID, "T_18.0.0", int32(newMode))
	case len(v) == 5 && v[4] == "switchEco":
		newMode := 0
		if body.(bool) {
			newMode = 1
		}
		return velisPlantDataSet(clID, "T_18.0.2", int32(newMode))
	case len(v) == 5 && v[4] == "plantSettings":
		if method == "POST" {
			return postMedPlantSettings(body, clID)
		}
		re := map[string]any{}
		if c, ok := clientMap[clID]; ok && c.params != nil {
			re["MedAntilegionellaOnOff"] = c.params["T_18.0.5"]
			re["MedMaxSetpointTemperature"] = c.params["T_18.1.3"] / 10
			re["MedMaxSetpointTemperatureMin"] = c.paramsLimits["T_18.1.3"].Min / 10
			re["MedMaxSetpointTemperatureMax"] = c.paramsLimits["T_18.1.3"].Max / 10
		}
		return re
	default:
		log.Printf("no route for path %s", path)
		return nil
	}
}

func postMedPlantSettings(body any, clID string) any {
	bodyMap := body.(map[string]any)
	for key, value := range bodyMap {
		if key == "MedMaxSetpointTemperature" {
			valueMap := value.(map[string]any)
			newTemp := valueMap["new"].(int) * 10
			return velisPlantDataSet(clID, "T_18.1.3", int32(newTemp))
		}
		if key == "MedAntilegionellaOnOff" {
			valueMap := value.(map[string]any)
			newMode := int(valueMap["new"].(float64))
			return velisPlantDataSet(clID, "T_18.0.5", int32(newMode))
		}
	}
	return nil
}

func postSePlantSettings(body any, clID string) any {
	bodyMap := body.(map[string]any)
	for key, value := range bodyMap {
		if key == "SeMaxSetpointTemperature" {
			valueMap := value.(map[string]any)
			newTemp := valueMap["new"].(int) * 10
			return velisPlantDataSet(clID, "T_22.1.2", int32(newTemp))
		}
		if key == "SeAntiCoolingTemperature" {
			valueMap := value.(map[string]any)
			newTemp := valueMap["new"].(int) * 10
			return velisPlantDataSet(clID, "T_22.1.4", int32(newTemp))
		}
		if key == "SeAntilegionellaOnOff" {
			valueMap := value.(map[string]any)
			newMode := int32(valueMap["new"].(float64))
			return velisPlantDataSet(clID, "T_22.0.1", newMode)
		}
		if key == "SePermanentBoostOnOff" {
			valueMap := value.(map[string]any)
			newMode := int32(valueMap["new"].(float64))
			return velisPlantDataSet(clID, "T_22.0.2", newMode)
		}
		if key == "SeNightModeOnOff" {
			valueMap := value.(map[string]any)
			newMode := int32(valueMap["new"].(float64))
			return velisPlantDataSet(clID, "T_22.0.4", newMode)
		}
		if key == "SeAntiCoolingOnOff" {
			valueMap := value.(map[string]any)
			newMode := int32(valueMap["new"].(float64))
			return velisPlantDataSet(clID, "T_22.0.5", newMode)
		}
	}
	return nil
}

func sePlantData(path string, body any, params URL.Values, method string) any {
	v := strings.Split(path, "/")
	clID := v[3]
	switch {
	case len(v) == 4:
		re := map[string]any{}
		if c, ok := clientMap[clID]; ok && c.params != nil {
			re["on"] = c.params["T_22.0.0"]
			re["mode"] = c.params["T_22.0.3"]
			re["boostReqTemp"] = c.params["T_22.1.0"] / 10
			re["reqTemp"] = c.params["T_22.1.3"] / 10
			re["heatReq"] = c.params["T_22.3.0"]
			re["procReqTemp"] = c.params["T_22.3.1"] / 10
			re["antiLeg"] = c.params["T_22.3.4"]
			re["temp"] = float32(c.params["T_22.3.6"]) / 10
			re["avShw"] = c.params["T_22.3.9"]

			re["gw"] = clID
		}
		return re
	case len(v) == 5 && v[4] == "temperature":
		bodyMap := body.(map[string]any)
		newTemp := int32(bodyMap["new"].(float64)) * 10
		return velisPlantDataSet(clID, "T_22.1.3", newTemp)
	case len(v) == 5 && v[4] == "mode":
		bodyMap := body.(map[string]any)
		newMode := int32(bodyMap["new"].(float64))
		return velisPlantDataSet(clID, "T_22.0.3", newMode)
	case len(v) == 5 && v[4] == "switch":
		newMode := 0
		if body.(bool) {
			newMode = 1
		}
		return velisPlantDataSet(clID, "T_22.0.0", int32(newMode))
	case len(v) == 5 && v[4] == "plantSettings":
		if method == "POST" {
			return postSePlantSettings(body, clID)
		}
		re := map[string]any{}
		if c, ok := clientMap[clID]; ok && c.params != nil {
			re["SeAntilegionellaOnOff"] = c.params["T_22.0.1"]
			re["SePermanentBoostOnOff"] = c.params["T_22.0.2"]
			re["SeNightModeOnOff"] = c.params["T_22.0.4"]
			re["SeAntiCoolingOnOff"] = c.params["T_22.0.5"]
			re["SeMaxSetpointTemperature"] = c.params["T_22.1.2"] / 10
			re["SeMaxSetpointTemperatureMin"] = c.paramsLimits["T_22.1.2"].Min / 10
			re["SeMaxSetpointTemperatureMax"] = c.paramsLimits["T_22.1.2"].Max / 10
			re["SeAntiCoolingTemperature"] = c.params["T_22.1.4"] / 10
			re["SeAntiCoolingTemperatureMin"] = c.paramsLimits["T_22.1.4"].Min / 10
			re["SeAntiCoolingTemperatureMax"] = c.paramsLimits["T_22.1.4"].Max / 10
		}
		return re
	default:
		log.Printf("no route for path %s", path)
		return nil
	}
}

func apiLogic() {
	apiToken = fmt.Sprintf("%v:%v", Config.Api_username, Config.Api_password)

	http.HandleFunc("/accounts/login", commonHandler(login))
	http.HandleFunc("/remote/plants", commonHandler(remotePlants))
	http.HandleFunc("/velis/plants", commonHandler(velisPlants))
	http.HandleFunc("/velis/medPlantData/", commonHandler(medPlantData))
	http.HandleFunc("/velis/sePlantData/", commonHandler(sePlantData))
	http.HandleFunc("/busErrors", commonHandler(busErrors))
	http.HandleFunc("/remote/plants/", commonHandler(features))
	http.HandleFunc("/remote/reports/", commonHandler(consumption))
	http.HandleFunc("/", commonHandler(defaultHandler))
	go func() {
		log.Printf("API server listening on %v", Config.Api_listener)
		if err := http.ListenAndServe(Config.Api_listener, nil); err != nil {
			log.Fatal(err)
		}
	}()
}
