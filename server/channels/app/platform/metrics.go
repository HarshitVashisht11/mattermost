// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package platform

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/channels/utils"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

const TimeToWaitForConnectionsToCloseOnServerShutdown = time.Second

type platformMetrics struct {
	server *http.Server
	router *mux.Router
	lock   sync.Mutex
	logger *mlog.Logger

	metricsImpl einterfaces.MetricsInterface

	cfgFn      func() *model.Config
	listenAddr string

	getPluginsEnv func() *plugin.Environment
}

// resetMetrics resets the metrics server. Clears the metrics if the metrics are disabled by the config.
func (ps *PlatformService) resetMetrics() error {
	if !*ps.Config().MetricsSettings.Enable {
		if ps.metrics != nil {
			return ps.metrics.stopMetricsServer()
		}
		return nil
	}

	if ps.metrics != nil {
		if err := ps.metrics.stopMetricsServer(); err != nil {
			return err
		}
	}

	ps.metrics = &platformMetrics{
		cfgFn:       ps.Config,
		metricsImpl: ps.metricsIFace,
		logger:      ps.logger,
		getPluginsEnv: func() *plugin.Environment {
			if ps.pluginEnv == nil {
				return nil
			}
			return ps.pluginEnv.GetPluginsEnvironment()
		},
	}

	if err := ps.metrics.initMetricsRouter(); err != nil {
		return err
	}

	if ps.metricsIFace != nil {
		ps.metricsIFace.Register()
	}

	return ps.metrics.startMetricsServer()
}

func (pm *platformMetrics) stopMetricsServer() error {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	if pm.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), TimeToWaitForConnectionsToCloseOnServerShutdown)
		defer cancel()

		if err := pm.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("could not shutdown metrics server: %v", err)
		}

		pm.logger.Info("Metrics and profiling server is stopped")
	}

	return nil
}

func (pm *platformMetrics) startMetricsServer() error {
	var notify chan struct{}
	pm.lock.Lock()
	defer func() {
		if notify != nil {
			<-notify
		}
		pm.lock.Unlock()
	}()

	l, err := net.Listen("tcp", *pm.cfgFn().MetricsSettings.ListenAddress)
	if err != nil {
		return err
	}

	notify = make(chan struct{})
	pm.server = &http.Server{
		Handler:      handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(pm.router),
		ReadTimeout:  time.Duration(*pm.cfgFn().ServiceSettings.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(*pm.cfgFn().ServiceSettings.WriteTimeout) * time.Second,
	}

	go func() {
		close(notify)
		if err := pm.server.Serve(l); err != nil && err != http.ErrServerClosed {
			pm.logger.Fatal(err.Error())
		}
	}()

	pm.listenAddr = l.Addr().String()
	pm.logger.Info("Metrics and profiling server is started", mlog.String("address", pm.listenAddr))
	return nil
}

func (pm *platformMetrics) initMetricsRouter() error {
	pm.router = mux.NewRouter()
	runtime.SetBlockProfileRate(*pm.cfgFn().MetricsSettings.BlockProfileRate)

	rootMetricsPage := `
			<html>
				<body>{{if .}}
					<div><a href="/metrics">Metrics</a></div>{{end}}
					<div><a href="/debug/pprof/">Profiling Root</a></div>
					<div><a href="/debug/pprof/cmdline">Profiling Command Line</a></div>
					<div><a href="/debug/pprof/symbol">Profiling Symbols</a></div>
					<div><a href="/debug/pprof/goroutine">Profiling Goroutines</a></div>
					<div><a href="/debug/pprof/heap">Profiling Heap</a></div>
					<div><a href="/debug/pprof/threadcreate">Profiling Threads</a></div>
					<div><a href="/debug/pprof/block">Profiling Blocking</a></div>
					<div><a href="/debug/pprof/trace">Profiling Execution Trace</a></div>
					<div><a href="/debug/pprof/profile">Profiling CPU</a></div>
					<div><a href="/plugins">Plugins Profiling</a></div>
				</body>
			</html>
		`
	metricsPageTmpl, err := template.New("rootMetricsPage").Parse(rootMetricsPage)
	if err != nil {
		return errors.Wrap(err, "failed to create template")
	}

	rootHandler := func(w http.ResponseWriter, r *http.Request) {
		pm.renderTemplate(metricsPageTmpl, r, w, pm.metricsImpl != nil)
	}

	pm.router.HandleFunc("/", rootHandler)
	pm.router.StrictSlash(true)

	pm.router.Handle("/debug", http.RedirectHandler("/", http.StatusMovedPermanently))
	pm.router.HandleFunc("/debug/pprof/", pprof.Index)
	pm.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pm.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pm.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pm.router.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Manually add support for paths linked to by index page at /debug/pprof/
	pm.router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	pm.router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	pm.router.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	pm.router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	pm.router.Handle("/debug/pprof/block", pprof.Handler("block"))
	pm.router.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))

	pluginsRouter := pm.router.PathPrefix("/plugins").Subrouter()
	pluginsRouter.HandleFunc("/", pm.serveListPluginsRequest)

	pluginMetricsPage := `
			<html>
				<body>
					<div><a href="debug/pprof/">Profiling Root</a></div>
					<div><a href="debug/pprof/cmdline">Profiling Command Line</a></div>
					<div><a href="debug/pprof/symbol">Profiling Symbols</a></div>
					<div><a href="debug/pprof/goroutine">Profiling Goroutines</a></div>
					<div><a href="debug/pprof/heap">Profiling Heap</a></div>
					<div><a href="debug/pprof/threadcreate">Profiling Threads</a></div>
					<div><a href="debug/pprof/block">Profiling Blocking</a></div>
					<div><a href="debug/pprof/trace">Profiling Execution Trace</a></div>
					<div><a href="/debug/pprof/profile">Profiling CPU</a></div>
				</body>
			</html>
		`

	pluginMetricsPageTmpl, err := template.New("pluginMetricsPage").Parse(pluginMetricsPage)
	if err != nil {
		return errors.Wrap(err, "failed to create template")
	}
	pluginRouter := pluginsRouter.PathPrefix("/{plugin_id:[A-Za-z0-9\\_\\-\\.]+}").Subrouter()
	pluginRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		pm.renderTemplate(pluginMetricsPageTmpl, r, w, nil)
	})

	// Plugins metrics route
	pluginsMetricsRoute := pluginRouter.PathPrefix("/metrics").Subrouter()
	pluginsMetricsRoute.HandleFunc("", pm.servePluginMetricsRequest)
	pluginsMetricsRoute.HandleFunc("/{anything:.*}", pm.servePluginMetricsRequest)

	// Plugins debug route
	debugRouter := pluginRouter.PathPrefix("/debug").Subrouter()
	debugRouter.StrictSlash(false)
	debugRouter.Handle("/debug", http.RedirectHandler("/", http.StatusMovedPermanently)) // TODO(hanzei): Maybe add this
	debugRouter.HandleFunc("/{anything:.*}", pm.servePluginDebugMetricsRequest)

	return nil
}

func (pm *platformMetrics) serveListPluginsRequest(w http.ResponseWriter, r *http.Request) {
	pluginsEnvironment := pm.getPluginsEnv()
	if pluginsEnvironment == nil {
		appErr := model.NewAppError("serveListPluginsRequest", "app.plugin.disabled.app_error",
			nil, "Enable plugins to serve plugin metric requests", http.StatusNotImplemented)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	bundles := pluginsEnvironment.Active()

	var ids []string
	for _, b := range bundles {
		ids = append(ids, b.Manifest.Id)
	}
	sort.Strings(ids)

	page := `
	<html>
		<body>
		{{range .}}
			<div><a href="/plugins/{{.}}">{{.}}</a></div>
		{{end}}
		</body>
	</html>
`
	metricsPageTmpl, err := template.New("page").Parse(page)
	if err != nil {
		//return errors.Wrap(err, "failed to create template")
		appErr := model.NewAppError("serveListPluginsRequest", "app.plugin.disabled.app_error",
			nil, "TODO", http.StatusNotImplemented)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	pm.renderTemplate(metricsPageTmpl, r, w, ids)
}

func (pm *platformMetrics) servePluginDebugMetricsRequest(w http.ResponseWriter, r *http.Request) {
	pluginID := mux.Vars(r)["plugin_id"]

	pluginsEnvironment := pm.getPluginsEnv()
	if pluginsEnvironment == nil {
		appErr := model.NewAppError("servePluginDebugMetricsRequest", "app.plugin.disabled.app_error",
			nil, "Enable plugins to serve plugin metric requests", http.StatusNotImplemented)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	subpath, err := utils.GetSubpathFromConfig(pm.cfgFn())
	if err != nil {
		appErr := model.NewAppError("servePluginDebugMetricsRequest", "app.plugin.subpath_parse.app_error",
			nil, "Failed to parse SiteURL subpath", http.StatusInternalServerError).Wrap(err)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	r.URL.Path = strings.TrimPrefix(r.URL.Path, path.Join(subpath, "plugins", pluginID, "debug"))

	// Passing an empty plugin context for the time being. To be decided whether we
	// should support forms of authentication in the future.
	pluginsEnvironment.ServeDebug(pluginID, &plugin.Context{}, w, r)
}

func (pm *platformMetrics) servePluginMetricsRequest(w http.ResponseWriter, r *http.Request) {
	pluginID := mux.Vars(r)["plugin_id"]

	pluginsEnvironment := pm.getPluginsEnv()
	if pluginsEnvironment == nil {
		appErr := model.NewAppError("ServePluginMetricsRequest", "app.plugin.disabled.app_error",
			nil, "Enable plugins to serve plugin metric requests", http.StatusNotImplemented)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	hooks, err := pluginsEnvironment.HooksForPlugin(pluginID)
	if err != nil {
		mlog.Debug("Access to route for non-existent plugin",
			mlog.String("missing_plugin_id", pluginID),
			mlog.String("url", r.URL.String()),
			mlog.Err(err))
		http.NotFound(w, r)
		return
	}

	subpath, err := utils.GetSubpathFromConfig(pm.cfgFn())
	if err != nil {
		appErr := model.NewAppError("ServePluginMetricsRequest", "app.plugin.subpath_parse.app_error",
			nil, "Failed to parse SiteURL subpath", http.StatusInternalServerError).Wrap(err)
		mlog.Error(appErr.Error())
		w.WriteHeader(appErr.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(appErr.ToJSON()))
		return
	}

	r.URL.Path = strings.TrimPrefix(r.URL.Path, path.Join(subpath, "plugins", pluginID, "metrics"))

	// Passing an empty plugin context for the time being. To be decided whether we
	// should support forms of authentication in the future.
	hooks.ServeMetrics(&plugin.Context{}, w, r)
}

func (pm *platformMetrics) renderTemplate(tmpl *template.Template, r *http.Request, w io.Writer, data any) {
	err := tmpl.Execute(w, data)
	if err != nil {
		pm.logger.Warn("Failed to debug metrics page",
			mlog.String("path", r.URL.Path),
			mlog.Err(err),
		)
	}
}

func (ps *PlatformService) HandleMetrics(route string, h http.Handler) {
	if ps.metrics != nil {
		ps.metrics.router.Handle(route, h)
	}
}

func (ps *PlatformService) RestartMetrics() error {
	return ps.resetMetrics()
}

func (ps *PlatformService) Metrics() einterfaces.MetricsInterface {
	if ps.metrics == nil {
		return nil
	}

	return ps.metricsIFace
}
