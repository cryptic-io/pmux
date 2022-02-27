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
stubborn-pinger ~ starting process
pinger          ~ starting process
stubborn-pinger › PING example.com (93.184.216.34) 56(84) bytes of data.
stubborn-pinger › 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=54 time=9.54 ms
stubborn-pinger ›
stubborn-pinger › --- example.com ping statistics ---
stubborn-pinger › 1 packets transmitted, 1 received, 0% packet loss, time 0ms
stubborn-pinger › rtt min/avg/max/mdev = 9.541/9.541/9.541/0.000 ms
pinger          › PING example.com (93.184.216.34) 56(84) bytes of data.
pinger          › 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=54 time=9.53 ms
pinger          ›
pinger          › --- example.com ping statistics ---
pinger          › 1 packets transmitted, 1 received, 0% packet loss, time 0ms
pinger          › rtt min/avg/max/mdev = 9.533/9.533/9.533/0.000 ms
pinger          › PING example.com (93.184.216.34) 56(84) bytes of data.
pinger          › 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=54 time=11.4 ms
pinger          ›
pinger          › --- example.com ping statistics ---
pinger          › 1 packets transmitted, 1 received, 0% packet loss, time 0ms
pinger          › rtt min/avg/max/mdev = 11.435/11.435/11.435/0.000 ms
stubborn-pinger › PING example.com (93.184.216.34) 56(84) bytes of data.
stubborn-pinger › 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=54 time=11.2 ms
stubborn-pinger ›
stubborn-pinger › --- example.com ping statistics ---
stubborn-pinger › 1 packets transmitted, 1 received, 0% packet loss, time 0ms
stubborn-pinger › rtt min/avg/max/mdev = 11.161/11.161/11.161/0.000 ms

... Ctrl-C ...

pmux            ~ interrupt signal received, killing all sub-processes
stubborn-pinger » i will never stop, you will have to SIGKILL me!
pinger          ~ exit code -1, process exited: signal: interrupt
pinger          ~ stopped process handler
stubborn-pinger › PING example.com (93.184.216.34) 56(84) bytes of data.
stubborn-pinger › 64 bytes from 93.184.216.34 (93.184.216.34): icmp_seq=1 ttl=54 time=14.8 ms
stubborn-pinger ›
stubborn-pinger › --- example.com ping statistics ---
stubborn-pinger › 1 packets transmitted, 1 received, 0% packet loss, time 0ms
stubborn-pinger › rtt min/avg/max/mdev = 14.793/14.793/14.793/0.000 ms
stubborn-pinger ~ forcefully killing process
stubborn-pinger ~ exit code -1, process exited: signal: killed
stubborn-pinger ~ stopped process handler
pmux            ~ exited gracefully, ciao!
```
