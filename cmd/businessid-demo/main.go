// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

// Command businessid-demo validates the identifiers given on the command line.
//
//	businessid-demo vat "BE 0123.456.749"
package main

import (
	"fmt"
	"os"

	businessid "github.com/libbusinessid/businessid-go"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: businessid-demo <kind> <value>")
		os.Exit(2)
	}
	report, err := businessid.New().Validate(businessid.Input{Kind: os.Args[1], Value: os.Args[2]})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("kind      %s\ncountry   %s\ncanonical %s\nformat    %s (%s)\nchecksum  %s (%s)\n",
		report.Kind, report.CountryCode, report.CanonicalValue,
		report.Format.Status, report.Format.Reason,
		report.Checksum.Status, report.Checksum.Reason)
}
