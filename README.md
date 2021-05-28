[![Sensu Bonsai Asset](https://img.shields.io/badge/Bonsai-Download%20Me-brightgreen.svg?colorB=89C967&logo=sensu)](https://bonsai.sensu.io/assets/sensu/sensu-multiplexer-check)
![Go Test](https://github.com/sensu/sensu-multiplexer-check/workflows/Go%20Test/badge.svg)
![goreleaser](https://github.com/sensu/sensu-multiplexer-check/workflows/goreleaser/badge.svg)

# Multiplexer Check

## Table of Contents
- [Overview](#overview)
- [Usage examples](#usage-examples)
- [Configuration](#configuration)
  - [Asset registration](#asset-registration)
  - [Check definition](#check-definition)
- [Installation from source](#installation-from-source)
- [Additional notes](#additional-notes)
- [Contributing](#contributing)

## Overview
The Multiplexer Check is a [Sensu Check][6] that allows to run a command multiple times with different 
arguments and generate separate Sensu events for each command execution.

The command calling arguments are controlled using either check or entity annotations.
Annotation keys are specially formatted to encode named argument groups, with each argument group
representing a separate iteration of the command to run

Note: The Sensu Check configurion must specify `stdin: true` for correct operation

## Usage examples

### sensu-multiplexer-check

#### Help output

```
Multiplexer Check

Usage:
  sensu-multiplexer-check [flags]
  sensu-multiplexer-check [command]

Available Commands:
  help        Help about any command
  version     Print the version number of this plugin

Flags:
  -p, --annotation-prefix string   Annotation key prefix to parse for command groups. (Required)
      --check-name-prefix string   prefix string to use in check name for each annotation group. (Optional) (default "multiplex_")
  -c, --command string             command executable to run. (Required)
  -a, --common-arguments string    common arguments for all annotation groups, appended to end of command. (Optional)
  -n, --dry-run                    dry run. Report generated events to stdout, but do not send them to events api. (Optional)
      --event-check string         json representation of substitute Sensu check to use in generated events. (Optional)
      --event-entity string        json representation of substitute Sensu entity to use in generated events. (Optional)
      --events-api string          Events API endpoint to use when generating events, can be overridden by endpoint json attribute of same name (default "http://localhost:3031/events")
  -h, --help                       help for sensu-multiplexer-check

Use "sensu-multiplexer-check [command] --help" for more information about a command.

```

#### Multiplexed http-check example

Here is a mock event structure that contains example entity and check annotations that will be processed
as four different argument groups for the `http-check` command, resulting in 4 iterations of the command being run.

test.json
```
{
  "check": {
    "metadata": {
      "annotations": {
        "http-check/args/check-test": "-u https://www.sensu.io/",
        "http-check/args/bad/url": "http://bonsai.sensu.io/"
        }
      }
  },
  "entity": {
    "metadata": {
      "annotations": {
        "http-check/args/entity-test": "-u https://docs.sensu.io/",
        "http-check/args/good/url": "http://bonsai.sensu.io/",
        "http-check/args/good/redirect-ok": ""
      }
    }
  }
}

```

Test innovation on the cmdline, using the local running sensu-agent events API:
```
$ cat test.json | ./sensu-multiplexer-check --command "http-check" \
--annotation-prefix "http-check/args" --events-api "http://localhost:3031/events"

Event Summary:
Event For Command: /usr/local/bin/http-check --redirect-ok
 Output: http-check CRITICAL: HTTP Status 403 for http://localhost:80/

 Status: 2
 Error: none
Event For Command: /usr/local/bin/http-check --url http://bonsai.sensu.io/
 Output: http-check WARNING: HTTP Status 301 for http://bonsai.sensu.io/  (redirects to https://bonsai.sensu.io/)

 Status: 1
 Error: none
Event For Command: /usr/local/bin/http-check --url http://bonsai.sensu.io/
 Output: http-check WARNING: HTTP Status 301 for http://bonsai.sensu.io/  (redirects to https://bonsai.sensu.io/)

 Status: 1
 Error: none
Event For Command: /user/local/bin/http-check -u https://docs.sensu.io/
 Output: http-check OK: HTTP Status 200 for https://docs.sensu.io/

 Status: 0
 Error: none
Event For Command: /user/local/bin/http-check -u https://www.sensu.io/
 Output: http-check OK: HTTP Status 200 for https://www.sensu.io/

 Status: 0
 Error: none
```

There are now 4 generated events corresponding to each of the argument groups 
```
$ sensuctl event list --field-selector 'event.check.name matches "multiplex_"'
     Entity              Check                                                            Output                                                    Status
 ────────────── ─────────────────────── ────────────────────────────────────────────────────────────────────────────────────────────────────────── ────────
  local_agent   multiplex_bad           http-check WARNING: HTTP Status 301 for http://bonsai.sensu.io/  (redirects to https://bonsai.sensu.io/)        1  
                                                                                                                                                                                                                                               
  local_agent   multiplex_check-test    http-check OK: HTTP Status 200 for https://www.sensu.io/                                                        0   
                                                                                                                                                                                                                                               
  local_agent   multiplex_entity-test   http-check OK: HTTP Status 200 for https://docs.sensu.io/                                                       0  
                                                                                                                                                                                                                                               
  local_agent   multiplex_good          http-check OK: HTTP Status 200 for https://bonsai.sensu.io/ (redirect from http://bonsai.sensu.io/)             0   
```


## Configuration

 

## Command Argument Annotation Formatting

The annotation keys inteded to be used as the multiplexed command arguments need to follow a special format
that includes an prefix string followed by an argument group string. The initial annotation prefix must be 
configured via the call to `sensu-multiplexer-check`. The argument group segment will be used as part of the 
generated event name.

### Individual Argument Annotation Key Format

Individual arguments for a argument group can be added using their own key.
These arguments must conform to the standard convention of double dash prefix for
long argument `--long_argument`
```
<annotation_prefix>/<argument_group>/<long_argument> : <argument_value>
```
Multiple individual argument annotations can be composed to form a complete
set of commandline arguments.

Ex:
```
"http-check/args/bonsai/url": "http://bonsai.sensu.io/"
"http-check/args/bonsai/redirecti-ok": ""
```
These annotations represents the argument string `--url http://bonsai.sensu.io/ --redirect-ok`
when using the annotation prefix `http-check/args`.  The argument group name
for the command will be `bonsai`.

### Combined Argument Annotation Key Format

For commands that have calling arguments that do not conform to the long
argument convention, you can construct a single command string by using an annotation
that ends with the argument group.

```
<annotation_prefix>/<argument_group> : <full_argument_string>
```
Individual argument annotations in an argument group will be appended to the cmdline argument list after
the value of this annotation.  

Ex:
```
"http-check/args/test": "-i -u https://localhost/"
"http-check/args/test/redirect-ok": ""
```

The full argument string for the `test` argument group would be `-i -u https://localhost/ --redirect-ok` 

### Asset registration

[Sensu Assets][10] are the best way to make use of this plugin. If you're not using an asset, please
consider doing so! If you're using sensuctl 5.13 with Sensu Backend 5.13 or later, you can use the
following command to add the asset:

```
sensuctl asset add sensu/sensu-multiplexer-check
```

If you're using an earlier version of sensuctl, you can find the asset on the [Bonsai Asset Index][https://bonsai.sensu.io/assets/sensu/sensu-multiplexer-check].

### Check definition

```yml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: multiplexed-http-check
  namespace: default
spec:
  command: sensu-multiplexer-check --command http-check --annotation-prefix "http-check/args"
  stdin: true
  subscriptions:
  - system
  runtime_assets:
  - sensu/sensu-multiplexer-check
  - sensu/http-checks
```

Note: `stdin: true` is required for the multiplexer to work. The command arguments are taken from the json passed into the check command by sensu-agent.

### Annotation Keyspace
Because this check requires stdin, all of the commandline arguments can be set via entity or check level annotations using 
the annotation prefix: `sensu.io/plugins/sensu-multiplexer-check/config`

#### Annotated Check definition example 

```yml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: multiplexed-http-check
  namespace: default
  annotations: 
    sensu.io/plugins/sensu-multiplexer-check/config/command: http-check
    sensu.io/plugins/sensu-multiplexer-check/config/annotation-prefix: http-check/args
spec:
  command: sensu-multiplexer-check
  stdin: true
  subscriptions:
  - system
  runtime_assets:
  - sensu/sensu-multiplexer-check
  - sensu/http-checks
```

## Installation from source

The preferred way of installing and deploying this plugin is to use it as an Asset. If you would
like to compile and install the plugin from source or contribute to it, download the latest version
or create an executable script from this source.

From the local path of the sensu-multiplexer-check repository:

```
go build
```

## Additional notes

## Contributing

For more information about contributing to this plugin, see [Contributing][1].

[1]: https://github.com/sensu/sensu-go/blob/master/CONTRIBUTING.md
[2]: https://github.com/sensu/sensu-plugin-sdk
[4]: https://github.com/sensu/check-plugin-template/blob/master/.github/workflows/release.yml
[5]: https://github.com/sensu/check-plugin-template/actions
[6]: https://docs.sensu.io/sensu-go/latest/reference/checks/
[7]: https://github.com/sensu/check-plugin-template/blob/master/main.go
[8]: https://bonsai.sensu.io/
[9]: https://github.com/sensu/sensu-plugin-tool
[10]: https://docs.sensu.io/sensu-go/latest/reference/assets/
