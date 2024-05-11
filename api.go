package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
)

var (
	apiListener = flag.String("api-listener", ":2080", "Listener address (e.g. :2080) of the API server")
	apiDebug    = flag.Bool("api-debug", false, "Debug handler for the API requests")
	apiUsername = flag.String("api-username", "", "Protect API with a username/password")
	apiPassword = flag.String("api-password", "", "Protect API with a username/password")
	apiToken    string
)

func commonHandler(handler func(path string, body any) any) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		if *apiDebug {
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
		result := handler(req.URL.Path, p)
		json.NewEncoder(w).Encode(result)
		if *apiDebug {
			log.Printf("API result: %+v", result)
		}
	}
}

func velisPlants(path string, body any) any {
	re := []any{}
	for clId, c := range clientMap {
		if c.birth == nil {
			continue
		}
		a := map[string]any{}
		for k, v := range c.birth {
			a[k] = v
		}
		a["gw"] = clId
		a["sn"] = c.birth["serial_number"]

		// TODO: map between all supported types
		if a["model_name"] == "Velis" {
			a["sys"] = 4
		}
		// TODO: map between all supported types
		if a["display_name"] == "REM4_EWH" {
			a["wheType"] = 2
		}
		re = append(re, a)
	}
	return re
}

func remotePlants(path string, body any) any {
	return []any{}
}

func defaultHandler(path string, body any) any {
	return map[string]any{}
}

func login(path string, body any) any {
	bmap, ok := body.(map[string]any)
	if !ok {
		return nil
	}
	if *apiUsername != "" {
		err := map[string]string{"error": "invalid username/password"}
		usr := bmap["usr"].(string)
		if usr != *apiUsername {
			return err
		}
		pwd := bmap["pwd"].(string)
		if pwd != *apiPassword {
			return err
		}
	}

	return map[string]any{"token": apiToken}
}

func velisPlantDataSet(clId, cat string, value int) any {
	b, err := putParams(cat, int32(value))
	if err == nil {
		err = server.Publish("$EDC/ari/"+clId+"/ar1/PUT/Menu/Par", b, false, 0)
	}
	if err != nil {
		log.Printf("error for %v %v %v: %v", clId, cat, value, err)
		return nil
	}
	return map[string]bool{"success": true}

}

func velisPlantData(path string, body any) any {
	v := strings.Split(path, "/")
	clID := v[3]
	switch {
	case len(v) == 4:
		re := map[string]any{}
		if c, ok := clientMap[clID]; ok && c.params != nil {
			re["mode"] = c.params["T_22.0.3"]
			re["temp"] = c.params["T_22.3.6"].(int32) / 10
			re["boostReqTemp"] = c.params["T_22.1.0"].(int32) / 10

			// TODO: these to may need to be swapped
			re["procReqTemp"] = c.params["T_22.3.1"].(int32) / 10
			re["reqTemp"] = c.params["T_22.1.3"].(int32) / 10

			re["gw"] = clID

			// TODO: figure out the others, e.g. on, heatReq, legionella

		}
		return re
	case len(v) == 5 && v[4] == "temperature":
		bodyMap := body.(map[string]any)
		newTemp := int(bodyMap["new"].(float64)) * 10
		return velisPlantDataSet(clID, "T_22.1.3", newTemp)
	case len(v) == 5 && v[4] == "mode":
		bodyMap := body.(map[string]any)
		newMode := int(bodyMap["new"].(float64))
		return velisPlantDataSet(clID, "T_22.0.3", newMode)
	default:
		return nil
	}
}

func apiLogic() {
	apiToken = fmt.Sprintf("%v:%v", *apiUsername, *apiPassword)

	http.HandleFunc("/accounts/login", commonHandler(login))
	http.HandleFunc("/remote/plants", commonHandler(remotePlants))
	http.HandleFunc("/velis/plants", commonHandler(velisPlants))
	http.HandleFunc("/velis/sePlantData/", commonHandler(velisPlantData))
	http.HandleFunc("/", commonHandler(defaultHandler))
	go func() {
		log.Printf("API server listening on %v", *apiListener)
		if err := http.ListenAndServe(*apiListener, nil); err != nil {
			log.Fatal(err)
		}
	}()
}
