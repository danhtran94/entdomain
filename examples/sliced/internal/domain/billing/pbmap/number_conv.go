package pbmap

import "strings"

// InvoiceNumberToProto and InvoiceNumberFromProto are the hand-written
// conversion functions named by Invoice.number's WithConversion annotation.
//
// The generated mapper (invoice_proto_gen.go) calls them unqualified, so they
// must live in this package: the mapper package of the entity's slice.
func InvoiceNumberToProto(n string) string { return strings.ToUpper(n) }

func InvoiceNumberFromProto(p string) string { return strings.ToUpper(p) }
