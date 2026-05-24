# caddy-exec

Caddy v2 module for running one-off commands.

## Installation

```
xcaddy build \
    --with github.com/abiosoft/caddy-exec
```

## Usage

Commands can be configured to be triggered globally during startup/shutdown or by via a route.

They can also be configured to run in the background or foreground and to be terminated after a timeout.

:warning: startup commands running on foreground will prevent Caddy from starting if they exit with an error.

### Caddyfile

```
exec [<matcher>] [<command> [<args...>]] {
    command     <command> [<args...>]
    args        <args...>
    directory   <directory>
    timeout     <timeout>
    log         <log output module>
    err_log     <log output module>
    foreground
    pass_thru
    startup
    shutdown
    run         <caddy handler>
}
```

- **matcher** - [Caddyfile matcher](https://caddyserver.com/docs/caddyfile/matchers). When set, this command runs when there is an http request at the current route or the specified matcher. You may leverage other matchers to protect the endpoint.
- **command** - command to run
- **args...** - command arguments. Supports Caddy template variables like {http.request.uri.path}, {http.request.host}, {http.request.url}, {http.request.method}, {http.request.query.*}, {http.request.header.*} for dynamic injection of request information.
- **directory** - directory to run the command from
- **timeout** - timeout to terminate the command's process. Default is `10s`. A timeout of `0` runs indefinitely.
- **log** - [Caddy log output module](https://caddyserver.com/docs/caddyfile/directives/log#output-modules) for standard output log. Defaults to `stderr`.
- **err_log** - [Caddy log output module](https://caddyserver.com/docs/caddyfile/directives/log#output-modules) for standard error log. Defaults to the value of `log` (standard output log).
- **foreground** - if present, runs the command in the foreground. For commands at http endpoints, the command will exit before the http request is responded to.
- **pass_thru** - if present, enables pass-thru mode, which continues to the next HTTP handler in the route instead of responding directly
- **startup** - if present, run the command at startup. Ignored in routes.
- **shutdown** - if present, run the command at shutdown. Ignored in routes.
- **run** - Caddy standard handler (e.g. reverse_proxy, redir) to execute before running the main command. If this command succeeds, the main command will be skipped. If this command fails, the main command will be executed. This allows you to configure dependencies or prerequisites before running the main command.

After the main command completes (whether executed or skipped), the run command will be executed again, allowing for post-execution actions like cleanup or notifications.

#### Example

`exec` can run at start via the [global](https://caddyserver.com/docs/caddyfile/options) directive.

```
{
  exec hugo generate --destination=/home/user/site/public {
      timeout 0 # don't timeout
  }
}
```

`exec` can be the last action of a route block.

```
route /update {
    ... # other directives e.g. for authentication
    exec git pull origin master {
        log file /var/logs/hugo.log
    }
}
```

You can also use the `run` parameter to execute a Caddy standard handler before running the main command:

```
route /update {
    # Run reverse_proxy first to ensure the backend is available
    exec update-site {
        command     sh
        args        -c "echo 'Updating site...'
        run         reverse_proxy http://localhost:8080
        log         file /var/logs/update.log
    }
}
```

You can use Caddy template variables in args to inject request information:

```
route /webhook {
    # Run a script that receives the request URL and path as parameters
    exec deploy.sh {
        command     /opt/deploy/deploy.sh
        args        "{http.request.url}" "{http.request.uri.path}" "{http.request.method}" "{http.request.query.id}"
        run         reverse_proxy http://localhost:3000
        log         file /var/logs/webhook.log
    }
}
```

The `run` command will be executed both before and after the main command:

```
route /cleanup {
    # Run a cleanup script after the main command completes
    exec process-data {
        command     /opt/process/process.sh
        args        "{http.request.remote.host}"
        run         log file /var/logs/health-check.log
        log         file /var/logs/process.log
    }
}
```

### Execution Flow

When `run` is specified, the execution flow is:

1. **First run**: Execute the `run` command (e.g., reverse_proxy)
   - If successful → Skip the main exec command
   - If failed → Execute the main exec command as fallback

2. **Main exec** (conditional): Only executed if the first run failed or no run specified

3. **Second run**: Always execute the `run` command again after the main command completes (or after first run if exec was skipped)

This allows you to:
- Use `run` as a health check before executing commands
- Automatically fallback to shell commands when the backend is unavailable
- Ensure cleanup or notification actions are always performed

### Notes

- The `run` parameter allows you to execute a Caddy standard handler (e.g., reverse_proxy, redir) before running the main shell command.
- If the `run` command fails, the main command will still be executed - this allows for graceful fallback behavior.
- The `run` parameter is particularly useful for ensuring that backend services are available before running commands that depend on them.
- After the main command completes, the `run` command will be executed again, allowing for post-execution actions like cleanup or notifications.
- Command args support Caddy template variables for dynamic injection of request information. Common variables include:
  - `{http.request.uri.path}` - Request path
  - `{http.request.host}` - Request hostname
  - `{http.request.url}` - Full URL
  - `{http.request.method}` - HTTP method
  - `{http.request.query.param_name}` - Query parameter value
  - `{http.request.header.Header-Name}` - Request header value
  - `{http.request.remote.host}` - Remote host address
  - `{http.request.scheme}` - Protocol scheme (http/https)

### API/JSON

As a top level app for `startup` and `shutdown` commands.

```jsonc
{
  "apps": {
    "http": { ... },
    // app configuration
    "exec": {
      // list of commands
      "commands": [
        // command configuration
        {
          // command to execute
          "command": "hugo",
          // [optional] command arguments. Supports Caddy template variables like {http.request.uri.path}, {http.request.host}, {http.request.url}, {http.request.method}, {http.request.query.*}, {http.request.header.*} for dynamic injection of request information.
          "args": [
            "generate",
            "--destination=/home/user/site/public"
          ],
          // when to run the command, can include 'startup' or 'shutdown'
          "at": ["startup"],

          // [optional] directory to run the command from. Default is the current directory.
          "directory": "",
          // [optional] if the command should run on the foreground. Default is false.
          "foreground": false,
          // [optional] if the middleware should respond directly or pass the request on to the next handler in the route. Default is false.
          "pass_thru": false,
          // [optional] timeout to terminate the command's process. Default is 10s.
          "timeout": "10s",
          // [optional] log output module config for standard output. Default is `stderr` module.
          "log": {
            "output": "file",
            "filename": "/var/logs/hugo.log"
          },
          // [optional] log output module config for standard error. Default is the value of `log`.
          "err_log": {
            "output": "stderr"
          },
          // [optional] Caddy standard handler (e.g. reverse_proxy, redir) to execute before running the main command
          "run": "reverse_proxy http://localhost:8080"
        }
      ]
    }
  }
}

```

As an handler within a route.

```jsonc

{
  ...
  "routes": [
    {
      "handle": [
        // exec configuration for an endpoint route
        {
          // required to inform caddy the handler is `exec`
          "handler": "exec",
          // command to execute
          "command": "git",
          // command arguments it's also possible to use
          // caddy variables like {http.request.uuid}
          "args": ["pull", "origin", "master", "# {http.request.uuid}"],

          // [optional] directory to run the command from. Default is the current directory.
          "directory": "/home/user/site/public",
          // [optional] if the command should run on the foreground. Default is false.
          "foreground": true,
          // [optional] if the middleware should respond directly or pass the request on to the next handler in the route. Default is false.
          "pass_thru": true,
          // [optional] timeout to terminate the command's process. Default is 10s.
          "timeout": "5s",
          // [optional] log output module config for standard output. Default is `stderr` module.
          "log": {
            "output": "file",
            "filename": "/var/logs/hugo.log"
          },
          // [optional] log output module config for standard error. Default is the value of `log`.
          "err_log": {
            "output": "stderr"
          },
          // [optional] Caddy standard handler (e.g. reverse_proxy, redir) to execute before running the main command
          "run": "reverse_proxy http://localhost:8080"
        }
      ],
      "match": [
        {
          "path": ["/update"]
        }
      ]
    }
  ]
}
```

## Dynamic Configuration

Caddy supports dynamic zero-downtime configuration reloads and it is possible to modify `exec`'s configurations at runtime.

`exec` intelligently determines when Caddy is starting and shutting down. i.e. startup and shutdown commands do not get triggered during configuration reload, only during Caddy's actual startup and shutdown.

## License

Apache 2

### Using with /autopilot

After the main command completes (whether executed or skipped), you may need to manually run the `/autopilot` skill if you want to continue with further automation. The `/autopilot` skill is a Claude Code feature and cannot be directly triggered from within the Caddy plugin.

For automation workflows, consider using the `run` parameter to execute scripts that trigger external automation tools, or use the `pass_thru` option to continue to a route that handles further processing.
