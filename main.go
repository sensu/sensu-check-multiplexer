package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sensu-community/sensu-plugin-sdk/sensu"
	"github.com/sensu/sensu-go/types"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Command          string
	CommonSuffixArgs string
	CommonPrefixArgs string
	AnnotationPrefix string
	CheckPrefix      string
	EventsAPI        string
	EventCheck       string
	EventEntity      string
	CreateEvent      bool
	DryRun           bool
}

var (
	argGroupMap = map[string]map[string]string{}
	argsMap     = map[string]string{}
	useStdin    = false
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
			Usage:     "command executable to run. (Required)",
			Value:     &plugin.Command,
		},
		&sensu.PluginConfigOption{
			Path:      "common-arguments",
			Env:       "MULTIPLEX_COMMON_ARGUMENTS",
			Argument:  "common-arguments",
			Shorthand: "a",
			Default:   "",
			Usage:     "common arguments for all annotation groups, appended to end of command. (Optional)",
			Value:     &plugin.CommonSuffixArgs,
		},
		&sensu.PluginConfigOption{
			Path:      "annotation-prefix",
			Env:       "MULTIPLEX_ANNOTATION_PREFIX",
			Argument:  "annotation-prefix",
			Shorthand: "p",
			Default:   "",
			Usage:     "Annotation key prefix to parse for command groups. (Required)",
			Value:     &plugin.AnnotationPrefix,
		},
		&sensu.PluginConfigOption{
			Path:     "event-check-prefix",
			Env:      "MULTIPLEX_CHECK_NAME_PREFIX",
			Argument: "check-name-prefix",
			Default:  "multiplex_",
			Usage:    "prefix string to use in check name for each annotation group. (Optional)",
			Value:    &plugin.CheckPrefix,
		},
		&sensu.PluginConfigOption{
			Path:     "event-entity",
			Env:      "MULTIPLEX_EVENT_ENTITY",
			Argument: "event-entity",
			Default:  "",
			Usage:    "json representation of substitute Sensu entity to use in generated events. (Optional)",
			Value:    &plugin.EventEntity,
		},
		&sensu.PluginConfigOption{
			Path:     "event-check",
			Env:      "MULTIPLEX_EVENT_CHECK",
			Argument: "event-check",
			Default:  "",
			Usage:    "json representation of substitute Sensu check to use in generated events. (Optional)",
			Value:    &plugin.EventCheck,
		},
		&sensu.PluginConfigOption{
			Path:      "dry-run",
			Argument:  "dry-run",
			Shorthand: "n",
			Default:   false,
			Usage:     "dry run. Report generated events to stdout, but do not send them to events api. (Optional)",
			Value:     &plugin.DryRun,
		},
		{
			Path:     "events-api",
			Env:      "",
			Argument: "events-api",
			Default:  "http://localhost:3031/events",
			Usage:    "Events API endpoint to use when generating events, can be overridden by endpoint json attribute of same name",
			Value:    &plugin.EventsAPI,
		},
	}
)

func main() {
	fi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Printf("Error accessing stdin: %v\n", err)
		panic(err)
	}
	//Check the Mode bitmask for Named Pipe to indicate stdin is connected
	if fi.Mode()&os.ModeNamedPipe != 0 {
		useStdin = true
	}

	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, useStdin)
	check.Execute()
}

func checkArgs(event *types.Event) (int, error) {
	if !useStdin {
		return sensu.CheckStateCritical, fmt.Errorf("Sensu event must be passed via stdin. If running under Sensu agent, please check the calling Sensu check definition and make sure stdin is enabled")
	}
	fields := strings.Fields(plugin.Command)
	if len(fields) == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--command or MULTIPLEX_COMMAND environment variable is blank")
	}
	plugin.Command = fields[0]
	if len(fields) > 1 {
		plugin.CommonPrefixArgs = strings.Join(fields[1:], ` `)
	}
	if len(plugin.Command) == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--command or MULTIPLEX_COMMAND environment variable is required")
	}
	if len(plugin.AnnotationPrefix) == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--annotation-prefix or MULTIPLEX_ANNOTATION_PREFIX environment variable is required")
	}
	fmt.Printf("Fields: %q\n", fields)
	fmt.Printf("Command: %s Args: %s\n", plugin.Command, plugin.CommonPrefixArgs)
	return sensu.CheckStateOK, nil
}

func executeCheck(event *types.Event) (int, error) {
	log.Println("executing check with --command", plugin.Command)
	log.Println("event", event)
	createCommandlines(event)
	for group, args := range argsMap {
		fmt.Printf("executing Command: %s Args: %s\n", plugin.Command, args)
		fmt.Printf("exeucting Group: %s Command: %s Args: %s\n", group, plugin.Command, args)
		cmd := exec.Command(plugin.Command, strings.Fields(args)...)
		stdoutStderr, err := cmd.CombinedOutput()
		if err != nil {
		}
		fmt.Printf("Group: %s Command: %s Args: %s\n Output: %s\n Err: %v\n", group, plugin.Command, args, stdoutStderr, err)
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
	if plugin.AnnotationPrefix == "" {
		return nil
	}
	prefix := path.Join(plugin.AnnotationPrefix) + "/"
	fmt.Printf("Prefix: %v\n", prefix)
	if event == nil {
		return nil
	}
	if event.Check != nil {
		processAnnotations(event.Check.Annotations, "Check", prefix)
	}
	if event.Entity != nil {
		processAnnotations(event.Entity.Annotations, "Entity", prefix)
	}
	fmt.Printf("Final Annotation Map: %q\n", argGroupMap)
	//loop over argument groups
	for group, arguments := range argGroupMap {
		//setup
		args := plugin.CommonPrefixArgs
		if len(argsMap[group]) > 0 {
			args = fmt.Sprintf("%s %s", args, argsMap[group])
		}
		for argument, value := range arguments {
			switch arg := argument; arg {
			default:
				//string leading dashes?
				//process long arguments
				args = fmt.Sprintf("%s --%s %s", args, arg, value)
			}
		}
		args = fmt.Sprintf("%s %s", args, plugin.CommonSuffixArgs)
		if len(plugin.Command) > 0 {
			argsMap[group] = args
		}
	}

	return nil
}

func processAnnotations(annotations map[string]string, annotationSource string, prefix string) {
	for key, value := range annotations {
		if strings.HasPrefix(key, prefix) {
			path := strings.SplitN(key, prefix, 2)[1]
			if len(path) > 0 {
				group := ""
				opt := ""
				subpath := strings.SplitN(path, "/", 2)
				if len(subpath) > 0 {
					group = subpath[0]
					groupMap := argGroupMap[group]
					if groupMap == nil { // initialize map
						fmt.Printf("%s initialize Map: %v\n", annotationSource, group)
						argGroupMap[group] = map[string]string{}
					}
					if group == path {
						argsMap[group] = value
						fmt.Printf("%s annotation: Group: %v Args Value: %v\n",
							annotationSource, group, argsMap[group])
					}
				}
				if len(subpath) == 2 {
					opt = subpath[1]
					if len(group) > 0 && len(opt) > 0 {
						argGroupMap[group][opt] = value
						fmt.Printf("%s annotation: Group: %v Opt: %v Value: %v\n",
							annotationSource, group, opt, argGroupMap[group][opt])
					}
				}
			}
		}
	}

}
