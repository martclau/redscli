// Package redscli implements the REDS API command-line client.
//
// The package keeps command parsing, HTTP transport, output formatting, and
// endpoint-specific handlers separate so the CLI remains easy to test without
// shelling out to the compiled binary.
package redscli
