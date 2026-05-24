package command

import (
	"encoding/json"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

var (
	_ caddy.Module                = (*Middleware)(nil)
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
)

func init() {
	caddy.RegisterModule(Middleware{})
}

// Middleware implements an HTTP handler that runs shell command.
type Middleware struct {
	Cmd
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.exec",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error { return m.Cmd.provision(ctx, m) }

// Validate implements caddy.Validator
func (m Middleware) Validate() error { return m.Cmd.validate() }

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	// replace per-request placeholders
	argv := make([]string, len(m.Args))
	for index, argument := range m.Args {
		argv[index] = repl.ReplaceAll(argument, "")
	}

	// First, try to execute the run command if specified
	var runSuccessful bool
	if m.Run != "" {
		// Create a temporary command for the run command
		runCmd := Cmd{
			Command: m.Run,
			// For the run command, we don't use args from the main command
			// but we need to set up logging similarly
			StdWriterRaw: m.StdWriterRaw,
			ErrWriterRaw: m.ErrWriterRaw,
			Timeout:      m.Timeout,
		}

		// Provision the run command
		if err := runCmd.provision(r.Context(), &m); err != nil {
			m.log.Error("Failed to provision run command", zap.Error(err))
		}

		// Try to execute the run command
		err := runCmd.run(nil)
		if err != nil {
			m.log.Info("Run command failed, will execute exec command", zap.Error(err))
			// Continue with the original exec command
		} else {
			m.log.Info("Run command executed successfully, skipping exec command")
			runSuccessful = true
		}
	}

	// Execute the original exec command only if run command failed or wasn't executed
	var err error
	if !runSuccessful && m.Run != "" {
		// Only execute exec if run command didn't succeed
		err = m.run(argv)
	} else if m.Run == "" {
		// If no run command specified, execute exec anyway
		err = m.run(argv)
	}

	// Execute the run command again after the main exec command
	if m.Run != "" {
		// Create a temporary command for the run command (second execution)
		runCmd := Cmd{
			Command: m.Run,
			// For the run command, we don't use args from the main command
			// but we need to set up logging similarly
			StdWriterRaw: m.StdWriterRaw,
			ErrWriterRaw: m.ErrWriterRaw,
			Timeout:      m.Timeout,
		}

		// Provision the run command
		if err := runCmd.provision(r.Context(), &m); err != nil {
			m.log.Error("Failed to provision second run command", zap.Error(err))
		}

		// Try to execute the run command again
		err2 := runCmd.run(nil)
		if err2 != nil {
			m.log.Info("Second run command failed", zap.Error(err2))
		} else {
			m.log.Info("Second run command executed successfully")
		}
	}

	if m.PassThru {
		if err != nil {
			m.log.Error(err.Error())
		}

		return next.ServeHTTP(w, r)
	}

	var resp struct {
		Status string `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	if err == nil {
		resp.Status = "success"
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		resp.Error = err.Error()
	}

	w.Header().Add("content-type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

// Cleanup implements caddy.Cleanup
// TODO: ensure all running processes are terminated.
func (m *Middleware) Cleanup() error {
	return nil
}