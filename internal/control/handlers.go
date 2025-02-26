// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package control

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/grundprinzip/spark-connect-proxy/internal/proxy"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func RegisterPromMetricsHandler(logger *slog.Logger, mux *mux.Router, reg *prometheus.Registry) {
	logger.Info("Registering Prometheus metrics handler")
	mux.Handle("/control/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
}

func RegisterSessionHandlers(logger *slog.Logger, mux *mux.Router, s *proxy.ProxyState) {
	logger.Info("Registering session handlers")
	mux.HandleFunc("/control/sessions",
		curry(s, logger, handleNewSession)).Methods("POST")
	mux.HandleFunc("/control/sessions/{id}",
		curry(s, logger, handleDeleteSesssion)).Methods("DELETE")
}

func curry(p *proxy.ProxyState, l *slog.Logger, fun func(p *proxy.ProxyState, l *slog.Logger, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fun(p, l, w, r)
	}
}
