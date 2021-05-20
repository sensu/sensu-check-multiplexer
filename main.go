package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/sensu-community/sensu-plugin-sdk/sensu"
	"github.com/sensu/sensu-go/types"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Example string
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-multiplexer-check",
			Short:    "Multiplexer Check",
			Keyspace: "sensu.io/plugins/sensu-multiplexer-check/config",
		},
	}

	options = []*sensu.PluginConfigOption{
		&sensu.PluginConfigOption{
			Path:      "example",
			Env:       "CHECK_EXAMPLE",
			Argument:  "example",
			Shorthand: "e",
			Default:   "",
			Usage:     "An example string configuration option",
			Value:     &plugin.Example,
		},
	}
)

func main() {
	useStdin := false
	fi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Printf("Error check stdin: %v\n", err)
		panic(err)
	}
	//Check the Mode bitmask for Named Pipe to indicate stdin is connected
	if fi.Mode()&os.ModeNamedPipe != 0 {
		log.Println("using stdin")
		useStdin = true
	}

	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, useStdin)
	check.Execute()
}

func checkArgs(event *types.Event) (int, error) {
	if len(plugin.Example) == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--example or CHECK_EXAMPLE environment variable is required")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *types.Event) (int, error) {
	log.Println("executing check with --example", plugin.Example)
	log.Println("event", event)
	argsArray(event)
	return sensu.CheckStateOK, nil
}

func argsArray(event *types.Event) error {
	optionMap := make(map[string]*sensu.PluginConfigOption)
	for _, opt := range options {
		if len(opt.Path) > 0 {
			optionMap[opt.Path] = opt
		}
	}
	if plugin.Keyspace == "" {
		return nil
	}
	prefix := path.Join(plugin.Keyspace, "args") + "/"
	fmt.Printf("Prefix: %v\n", prefix)
	if event == nil {
		return nil
	}
	if event.Check != nil {
		for key := range event.Check.Annotations {
			if strings.HasPrefix(key, prefix) {
				path := strings.SplitN(key, prefix, 2)[1]
				if len(path) > 0 {
					subpath := strings.SplitN(path, "/", 2)
					group := subpath[0]
					opt := subpath[1]
					if len(group) > 0 {
						fmt.Printf("Check annotation Group: %v Opt: %v\n", group, opt)
					}
				}
			}
		}
	}
	if event.Entity != nil {
		for key := range event.Entity.Annotations {
			if strings.HasPrefix(key, prefix) {
				path := strings.SplitN(key, prefix, 2)[1]
				if len(path) > 0 {
					subpath := strings.SplitN(path, "/", 2)
					group := subpath[0]
					opt := subpath[1]
					if len(group) > 0 {
						fmt.Printf("Entity annotation Group: %v Opt: %v\n", group, opt)
					}
				}
			}
		}
	}
	return nil
}
