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
	Command     string
	CommonArgs  string
	CheckPrefix string
	CreateEvent bool
	DryRun      bool
}

var (
	argGroupMap = map[string]map[string]string{}
	commandMap  = map[string]string{}
	plugin      = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-multiplexer-check",
			Short:    "Multiplexer Check",
			Keyspace: "sensu.io/plugins/sensu-multiplexer-check/config",
		},
	}

	options = []*sensu.PluginConfigOption{
		&sensu.PluginConfigOption{
			Path:      "command",
			Env:       "MULTIPLEX_COMMAND",
			Argument:  "command",
			Shorthand: "c",
			Default:   "",
			Usage:     "command to run",
			Value:     &plugin.Command,
		},
		&sensu.PluginConfigOption{
			Path:      "common-arguments",
			Env:       "MULTIPLEX_COMMON_ARGUMENTS",
			Argument:  "common-arguments",
			Shorthand: "a",
			Default:   "",
			Usage:     "common arguments for all annotation groups",
			Value:     &plugin.CommonArgs,
		},
		&sensu.PluginConfigOption{
			Path:      "event-check-prefix",
			Env:       "MULTIPLEX_EVENT_CHECK_PREFIX",
			Argument:  "event-check-prefix",
			Shorthand: "p",
			Default:   "multiplex_",
			Usage:     "prefix string to use in generated event",
			Value:     &plugin.CheckPrefix,
		},
		&sensu.PluginConfigOption{
			Path:      "create-event",
			Env:       "MULTIPLEX_CREATE_EVENT",
			Argument:  "create-event",
			Shorthand: "",
			Default:   false,
			Usage:     "create events",
			Value:     &plugin.CreateEvent,
		},
		&sensu.PluginConfigOption{
			Path:      "dry-run",
			Argument:  "dry-run",
			Shorthand: "n",
			Default:   false,
			Usage:     "dry run",
			Value:     &plugin.DryRun,
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
	/*
		if len(plugin.Command) == 0 {
			return sensu.CheckStateWarning, fmt.Errorf("--command or CHECK_COMMAND environment variable is required")
		}
	*/
	return sensu.CheckStateOK, nil
}

func executeCheck(event *types.Event) (int, error) {
	log.Println("executing check with --command", plugin.Command)
	log.Println("event", event)
	createCommandlines(event)
	for group, cmdline := range commandMap {
		fmt.Printf("Group: %s Cmdline: %s\n", group, cmdline)
	}
	return sensu.CheckStateOK, nil
}

func createCommandlines(event *types.Event) error {
	//map with string keys and values of a map with string keys and string values
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
		processAnnotations(event.Check.Annotations, "Check")
	}
	if event.Entity != nil {
		processAnnotations(event.Entity.Annotations, "Entity")
	}
	fmt.Printf("Final Annotation Map: %q\n", argGroupMap)
	//loop over argument groups
	for group, arguments := range argGroupMap {
		//setup
		command := plugin.Command
		args := plugin.CommonArgs
		for argument, value := range arguments {
			switch arg := argument; arg {
			case "command":
				command = value
			default:
				args = fmt.Sprintf("%s --%s %s", args, arg, value)
			}
		}
		if len(command) > 0 {
			cmdline := command + args
			commandMap[group] = cmdline
		}
	}

	return nil
}

func processAnnotations(annotations map[string]string, annotationSource string) {
	prefix := path.Join(plugin.Keyspace, "args") + "/"
	for key, value := range annotations {
		if strings.HasPrefix(key, prefix) {
			path := strings.SplitN(key, prefix, 2)[1]
			if len(path) > 0 {
				subpath := strings.SplitN(path, "/", 2)
				group := subpath[0]
				opt := subpath[1]
				if len(group) > 0 && len(opt) > 0 {
					groupMap := argGroupMap[group]
					if groupMap == nil { // initialize map
						fmt.Printf("%s initialize Map: %v\n", annotationSource, group)
						argGroupMap[group] = map[string]string{}
						groupMap = argGroupMap[group]
					}
					argGroupMap[group][opt] = value
					fmt.Printf("%s annotation: Group: %v Opt: %v Value: %v\n",
						annotationSource, group, opt, argGroupMap[group][opt])
				} else {
				}
			}
		}
	}

}
