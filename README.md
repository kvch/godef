# godef

`godef` is a tool for finding the definition of a symbol in Go source code. It can read input from a file, stdin, or the current acme window and provide information about the symbol's definition, type, and other relevant details. It supports various output formats and debugging features.

## Installation

To install `godef`, you need to have Go installed. Then, use the following command:

```sh
go install github.com/kvch/godef@latest
```

## Usage

```sh
godef [flags] [expr]
```

### Flags

- `-i`: Read file from stdin.
- `-o int`: File offset of identifier in stdin.
- `-debug`: Enable debug mode.
- `-t`: Print type information.
- `-a`: Print public type and member information.
- `-A`: Print all type and member information.
- `-f string`: Go source filename.
- `-acme`: Use current acme window.
- `-json`: Output location in JSON format (ignores `-t` flag).
- `-cpuprofile string`: Write CPU profile to this file.
- `-memprofile string`: Write memory profile to this file.
- `-trace string`: Write trace log to this file.

### Examples

#### Find the definition of a symbol in a file

```sh
godef -f example.go -o 123
```

#### Read from stdin

```sh
cat example.go | godef -i -o 123
```

#### Use in an acme window

```sh
godef -acme
```

#### Output in JSON format

```sh
godef -f example.go -o 123 -json
```

### Debugging

To enable debugging, use the `-debug` flag. This will print additional debug information to help diagnose issues.

### Profiling

To profile the CPU or memory usage of `godef`, use the `-cpuprofile` and `-memprofile` flags respectively. These will write profile data to the specified files.

```sh
godef -f example.go -o 123 -cpuprofile cpu.prof
godef -f example.go -o 123 -memprofile mem.prof
```

### Tracing

To enable tracing, use the `-trace` flag. This will write trace data to the specified file.

```sh
godef -f example.go -o 123 -trace trace.out
```

To view the trace, use the following command:

```sh
go tool trace view trace.out
```

## License

This project is licensed under the BSD-3-Clause license. See the [LICENSE](LICENSE) file for details.
