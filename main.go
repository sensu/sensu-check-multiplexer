package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

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
	CheckNamePrefix  string
	EventsAPI        string
	EventCheck       string
	EventEntity      string
	CreateEvent      bool
	DryRun           bool
}

type Command struct {
	CommandString string
	Status        int
	Output        string
	CheckName     string
	CommandError  error
	EventError    error
}

var (
	commands    []*Command
	argGroupMap = map[string]map[string]string{}
	argsMap     = map[string]string{}
	useStdin    = false
	plugin      = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-check-multiplexer",
			Short:    "Check Multiplexer",
			Keyspace: "sensu.io/plugins/sensu-check-multiplexer/config",
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
			Path:     "check-name-prefix",
			Env:      "MULTIPLEX_CHECK_NAME_PREFIX",
			Argument: "check-name-prefix",
			Default:  "multiplex_",
			Usage:    "prefix string to use in check name for each annotation group. (Optional)",
			Value:    &plugin.CheckNamePrefix,
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
	if plugin.DryRun {
		fmt.Printf("Command: %s Common Args: %s\n", plugin.Command, plugin.CommonPrefixArgs)
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *types.Event) (int, error) {
	if plugin.DryRun {
		fmt.Printf("\nDry-run Output\n")
	}
	//Populate the argsMap that forms the basis of commands to execute
	err := createCommandlines(event)
	if err != nil {
		return sensu.CheckStateCritical, err
	}
	// Execute commands in parallel
	var wg sync.WaitGroup
	for group := range argsMap {
		wg.Add(1)
		go func(group string, wg *sync.WaitGroup) {
			// run command
			c := new(Command)
			c.Run(group)
			// generate event
			c.EventError = c.generateEvent(event)
			// append command into commands array
			commands = append(commands, c)
			defer wg.Done()
		}(group, &wg)
		if plugin.DryRun {
			wg.Wait()
		}
	}
	// Wait for all commands to finish executing
	wg.Wait()
	if plugin.DryRun {
		fmt.Printf("\n\nNormal Output\n")
	}
	// Build Output Summary
	fmt.Printf("Event Summary:\n")
	var eventError bool
	for _, c := range commands {
		var errString string
		if c.EventError == nil {
			errString = "none"
		} else {
			eventError = true
			errString = fmt.Sprintf("%v", c.EventError)
		}
		fmt.Printf("Event For Command: %s\n Output: %s\n Status: %v\n Error: %v\n", c.CommandString, c.Output, c.Status, errString)
	}
	if eventError {
		return sensu.CheckStateCritical, nil
	} else {
		return sensu.CheckStateOK, nil
	}
}
func (c *Command) Run(group string) {
	args := argsMap[group]
	cmd := exec.Command(plugin.Command, strings.Fields(args)...)
	output, err := cmd.CombinedOutput()
	c.CommandString = cmd.String()
	c.CommandError = err
	c.CheckName = fmt.Sprintf("%s%s", plugin.CheckNamePrefix, group)
	if c.CommandError != nil && cmd.ProcessState == nil {
		c.Output = fmt.Sprintf("Unknown error running command: %s", c.CommandError)
		c.Status = 3
	} else {
		c.Output = string(output)
		c.Status = cmd.ProcessState.ExitCode()
	}
	if plugin.DryRun {
		fmt.Printf("Ran Command: %#v\n Status: %d\n Err: %v\n", c.CommandString, c.Status, c.CommandError)
	}

}

func (c *Command) generateEvent(event *types.Event) error {
	event.Check.Name = c.CheckName
	event.Check.Status = uint32(c.Status)
	event.Check.Output = c.Output
	eventJSON, err := json.Marshal(event)
	if err != nil {
		fmt.Printf("Create event failed with error %s\n", err)
		return err
	}
	if plugin.DryRun {
		fmt.Printf("Event For Command: %s\n", c.CommandString)
		fmt.Printf("  Check Name: %s\n", event.Check.Name)
		fmt.Printf("  Check Status: %v\n", event.Check.Status)
		fmt.Printf("  Check Output: %s\n", event.Check.Output)
		fmt.Printf("  Event API: %s\n  Event Data: %s\n", plugin.EventsAPI, string(eventJSON))
	} else {
		_, err = http.Post(plugin.EventsAPI, "application/json", bytes.NewBuffer(eventJSON))
		if err != nil {
			fmt.Printf("The HTTP request to create event failed with error %s\n", err)
			return err
		}
	}
	return nil
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
	if plugin.DryRun {
		fmt.Printf("Annotation Prefix: %v\n", prefix)
	}
	if event == nil {
		return nil
	}
	if event.Check != nil {
		processAnnotations(event.Check.Annotations, "Check", prefix)
	}
	if event.Entity != nil {
		processAnnotations(event.Entity.Annotations, "Entity", prefix)
	}
	if plugin.DryRun {
		fmt.Printf("Final Annotation Map: %q\n", argGroupMap)
	}
	//loop over argument groups
	for group, arguments := range argGroupMap {
		// start with common prefix args stripped from the command
		args := plugin.CommonPrefixArgs
		// if argsMap[group] already filled just use it do not process individual annotation option values
		if len(argsMap[group]) > 0 {
			args = fmt.Sprintf("%s %s", args, argsMap[group])
		} else {
			for argument, value := range arguments {
				args = fmt.Sprintf("%s --%s %s", args, argument, value)
			}
		}
		// add common suffix arguments
		args = fmt.Sprintf("%s %s", args, plugin.CommonSuffixArgs)
		// store in argsMap
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
						if plugin.DryRun {
							fmt.Printf("%s initialize Map: %v\n", annotationSource, group)
						}
						argGroupMap[group] = map[string]string{}
					}
					if group == path {
						argsMap[group] = value
						if plugin.DryRun {
							fmt.Printf("%s annotation: Group: %v Args Value: %v\n",
								annotationSource, group, argsMap[group])
						}
					}
				}
				if len(subpath) == 2 {
					opt = subpath[1]
					if len(group) > 0 && len(opt) > 0 {
						argGroupMap[group][opt] = value
						if plugin.DryRun {
							fmt.Printf("%s annotation: Group: %v Opt: %v Value: %v\n",
								annotationSource, group, opt, argGroupMap[group][opt])
						}
					}
				}
			}
		}
	}

}
