# pmux

A dumb simple user-space process manager, for use in composing multiple
processes together into a single runable frontend.

Features include (and are limited to):

* Coalesces all stdout and stderr streams of all sub-processes into a single
  stdout stream (with timestamps and process names prefixing each line).

* Propagates interrupt signal to sub-processes, and waits a configurable amount
  of time before SIGKILLing those which don't exit themselves.

* Will restart processes which unexpectedly exit, with an exponential backoff
  delay for those which repeatedly exit.

* Configurable timestamp format.

That's it. If it's not listed then pmux can't do it.

## Usage

To build you just `go build .` within the directory.

To run you do `pmux -c pmux.yml`. If `-c` isn't provided then pmux will look for
`pmux.yml` in the pwd. A config file is required.

## Example

This repo contains [an example config file](pmux-example.yml), which shows off
all possible configuration options.

The stdoutput from this example config looks something like this:

```
2021-09-21T16:32:48.513-06:00 | stubborn-pinger | starting process
2021-09-21T16:32:48.513-06:00 | pinger          | starting process
2021-09-21T16:32:48.532-06:00 > pinger          > PING example.com (93.184.216.34) 56(84) bytes of data.
2021-09-21T16:32:48.532-06:00 > pinger          > 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=55 time=14.1 ms
2021-09-21T16:32:48.532-06:00 > pinger          >
2021-09-21T16:32:48.532-06:00 > pinger          > --- example.com ping statistics ---
2021-09-21T16:32:48.532-06:00 > pinger          > 1 packets transmitted, 1 received, 0% packet loss, time 0ms
2021-09-21T16:32:48.532-06:00 > pinger          > rtt min/avg/max/mdev = 14.091/14.091/14.091/0.000 ms
2021-09-21T16:32:48.532-06:00 > stubborn-pinger > PING example.com (93.184.216.34) 56(84) bytes of data.
2021-09-21T16:32:48.532-06:00 > stubborn-pinger > 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=55 time=14.2 ms
2021-09-21T16:32:48.532-06:00 > stubborn-pinger >
2021-09-21T16:32:48.532-06:00 > stubborn-pinger > --- example.com ping statistics ---
2021-09-21T16:32:48.532-06:00 > stubborn-pinger > 1 packets transmitted, 1 received, 0% packet loss, time 0ms
2021-09-21T16:32:48.532-06:00 > stubborn-pinger > rtt min/avg/max/mdev = 14.154/14.154/14.154/0.000 ms
2021-09-21T16:32:49.548-06:00 > pinger          > PING example.com (93.184.216.34) 56(84) bytes of data.
2021-09-21T16:32:49.548-06:00 > pinger          > 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=55 time=10.5 ms
2021-09-21T16:32:49.548-06:00 > pinger          >
2021-09-21T16:32:49.548-06:00 > pinger          > --- example.com ping statistics ---
2021-09-21T16:32:49.548-06:00 > pinger          > 1 packets transmitted, 1 received, 0% packet loss, time 0ms
2021-09-21T16:32:49.548-06:00 > pinger          > rtt min/avg/max/mdev = 10.451/10.451/10.451/0.000 ms
2021-09-21T16:32:49.553-06:00 > stubborn-pinger > PING example.com (93.184.216.34) 56(84) bytes of data.
2021-09-21T16:32:49.553-06:00 > stubborn-pinger > 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=55 time=15.3 ms
2021-09-21T16:32:49.553-06:00 > stubborn-pinger >
2021-09-21T16:32:49.553-06:00 > stubborn-pinger > --- example.com ping statistics ---
2021-09-21T16:32:49.553-06:00 > stubborn-pinger > 1 packets transmitted, 1 received, 0% packet loss, time 0ms

... Ctrl-C

^C2021-09-21T16:32:50.894-06:00 | pmux            | interrupt signal received, killing all sub-processes
2021-09-21T16:32:50.895-06:00 > stubborn-pinger > i will never stop, you will have to SIGKILL me!
2021-09-21T16:32:50.895-06:00 | pinger          | process exited: signal: interrupt
2021-09-21T16:32:50.895-06:00 | pinger          | stopped process handler
2021-09-21T16:32:50.910-06:00 > stubborn-pinger > PING example.com (93.184.216.34) 56(84) bytes of data.
2021-09-21T16:32:50.910-06:00 > stubborn-pinger > 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=55 time=11.4 ms
2021-09-21T16:32:50.910-06:00 > stubborn-pinger >
2021-09-21T16:32:50.910-06:00 > stubborn-pinger > --- example.com ping statistics ---
2021-09-21T16:32:50.910-06:00 > stubborn-pinger > 1 packets transmitted, 1 received, 0% packet loss, time 0ms
2021-09-21T16:32:50.910-06:00 > stubborn-pinger > rtt min/avg/max/mdev = 11.369/11.369/11.369/0.000 ms
2021-09-21T16:32:51.895-06:00 | stubborn-pinger | forcefully killing process
2021-09-21T16:32:51.912-06:00 | stubborn-pinger | process exited: signal: killed
2021-09-21T16:32:51.912-06:00 | stubborn-pinger | stopped process handler
2021-09-21T16:32:51.912-06:00 | pmux            | exited gracefully, ciao!
```
