/*
 * Johan Stenstam, johan.stenstam@internetstiftelsen.se
 */
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"

	"github.com/dnstapir/tapir"
)

func APIcommand(conf *Config) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		decoder := json.NewDecoder(r.Body)
		var cp tapir.CommandPost
		err := decoder.Decode(&cp)
		if err != nil {
			log.Println("APICommand: error decoding command post:", err)
		}

		log.Printf("API: received /command request (cmd: %s) from %s.\n",
			cp.Command, r.RemoteAddr)

		resp := tapir.CommandResponse{}

		defer func() {
			// log.Printf("defer: resp: %v", resp)
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(resp)
			if err != nil {
				log.Printf("Error from json encoder: %v", err)
				log.Printf("resp: %v", resp)
			}
		}()

		switch cp.Command {
		case "status":
			log.Printf("Daemon status inquiry\n")

			resp = tapir.CommandResponse{
				Status: "ok", // only status we know, so far
				Msg:    "We're happy, but send more cookies",
			}

		case "stop":
			log.Printf("Daemon instructed to stop\n")
			resp = tapir.CommandResponse{
				Status: "stopping",
				Msg:    "Daemon was happy, but now winding down",
			}
			log.Printf("Stopping MQTT engine\n")
			conf.MqttHandler.Stop()
			//			conf.Internal.APIStopCh <- struct{}{}

		case "mqtt-start":
			conf.MqttHandler.Start()
			resp.Msg = "MQTT engine started"

		case "mqtt-stop":
			conf.MqttHandler.Stop()
			resp.Msg = "MQTT engine stopped"

		default:
			resp.Error = true
			resp.ErrorMsg = fmt.Sprintf("Unknown command: %s", cp.Command)
		}
	}
}

func APIdebug(conf *Config) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		resp := tapir.DebugResponse{
			Status: "ok", // only status we know, so far
			Msg:    "We're happy, but send more cookies",
		}

		defer func() {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(resp)
			if err != nil {
				log.Printf("Error from json encoder: %v", err)
				log.Printf("resp: %v", resp)
			}
		}()

		decoder := json.NewDecoder(r.Body)
		var dp tapir.DebugPost
		err := decoder.Decode(&dp)
		if err != nil {
			log.Println("APICdebug: error decoding debug post:", err)
		}

		log.Printf("API: received /debug request (cmd: %s) from %s.\n",
			dp.Command, r.RemoteAddr)

		switch dp.Command {

		default:
			resp.ErrorMsg = fmt.Sprintf("Unknown command: %s", dp.Command)
			resp.Error = true
		}
	}
}

// func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
func APIstatus(conf *Config) func(w http.ResponseWriter, r *http.Request) {
	mqttHandler := conf.MqttHandler

	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("TAPIR-SLOGGER Received /status request")
		status := mqttHandler.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

func SetupRouter(conf *Config) *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	sr := r.PathPrefix("/api/v1").Headers("X-API-Key", viper.GetString("apiserver.key")).Subrouter()
	sr.HandleFunc("/ping", tapir.APIping("tapir-slogger", conf.BootTime)).Methods("POST")
	sr.HandleFunc("/command", APIcommand(conf)).Methods("POST")
	sr.HandleFunc("/status", APIstatus(conf)).Methods("POST")
	sr.HandleFunc("/debug", APIdebug(conf)).Methods("POST")
	// sr.HandleFunc("/show/api", tapir.APIshowAPI(r)).Methods("GET")

	return r
}

func walkRoutes(router *mux.Router, address string) {
	log.Printf("Defined API endpoints for router on: %s\n", address)

	walker := func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		path, _ := route.GetPathTemplate()
		methods, _ := route.GetMethods()
		for m := range methods {
			log.Printf("%-6s %s\n", methods[m], path)
		}
		return nil
	}
	if err := router.Walk(walker); err != nil {
		log.Panicf("Logging err: %s\n", err.Error())
	}
	//	return nil
}

// In practice APIdispatcher doesn't need a termination signal, as it will
// just sit inside http.ListenAndServe, but we keep it for symmetry.
func APIhandler(conf *Config, done <-chan struct{}) {
	router := SetupRouter(conf)

	walkRoutes(router, viper.GetString("apiserver.address"))
	log.Println("")

	addresses := viper.GetStringSlice("apiserver.addresses")
	tlsaddresses := viper.GetStringSlice("apiserver.tlsaddresses")
	certfile := viper.GetString("certs.tapir-slogger.cert")
	if certfile == "" {
		log.Printf("*** APIhandler: Error: TLS cert file not specified under key certs.tapir-slogger.cert")
	}
	keyfile := viper.GetString("certs.tapir-slogger.key")
	if keyfile == "" {
		log.Printf("*** APIhandler: Error: TLS key file not specified under key certs.tapir-slogger.key")
	}

	tlspossible := true

	_, err := os.Stat(certfile)
	if os.IsNotExist(err) {
		log.Printf("*** APIhandler: Error: TLS cert file \"%s\" does not exist", certfile)
		tlspossible = false
	}
	_, err = os.Stat(keyfile)
	if os.IsNotExist(err) {
		log.Printf("*** APIhandler: Error: TLS key file \"%s\" does not exist", keyfile)
		tlspossible = false
	}

	tlsConfig, err := tapir.NewServerConfig(viper.GetString("certs.cacertfile"), tls.VerifyClientCertIfGiven)
	// Alternatives are: tls.RequireAndVerifyClientCert, tls.VerifyClientCertIfGiven,
	// tls.RequireAnyClientCert, tls.RequestClientCert, tls.NoClientCert

	if err != nil {
		TEMExiter("Error creating API server tls config: %v\n", err)
	}

	var wg sync.WaitGroup

	// log.Println("*** API: Starting API dispatcher #1. Listening on", address)

	if len(addresses) > 0 {
		for idx, address := range addresses {
			wg.Add(1)
			go func(wg *sync.WaitGroup) {
				apiServer := &http.Server{
					Addr:         address,
					Handler:      router,
					ReadTimeout:  10 * time.Second,
					WriteTimeout: 10 * time.Second,
				}

				log.Printf("*** API: Starting API dispatcher #%d. Listening on %s", idx+1, address)
				wg.Done()
				TEMExiter(apiServer.ListenAndServe())
			}(&wg)
		}
	} else {
		log.Printf("*** API: APIdispatcher: Error: No API addresses specified.\n")
	}

	if len(tlsaddresses) > 0 {
		if tlspossible {
			for idx, tlsaddress := range tlsaddresses {
				wg.Add(1)
				go func(wg *sync.WaitGroup) {
					tlsServer := &http.Server{
						Addr:         tlsaddress,
						Handler:      router,
						TLSConfig:    tlsConfig,
						ReadTimeout:  10 * time.Second,
						WriteTimeout: 10 * time.Second,
					}
					log.Printf("*** API: Starting TLS API dispatcher #%d. Listening on %s", idx+1, tlsaddress)
					wg.Done()
					TEMExiter(tlsServer.ListenAndServeTLS(certfile, keyfile))
				}(&wg)
			}
		} else {
			log.Printf("*** API: APIdispatcher: Error: Cannot provide TLS service without cert and key files.\n")
		}
	} else {
		log.Printf("*** API: APIdispatcher: Error: No TLS API addresses specified.\n")
	}

	wg.Wait()
	log.Println("API dispatcher: unclear how to stop the http server nicely.")
}
