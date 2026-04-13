package scip

import (
	"fmt"

	scippb "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// StreamConvertedDocuments is like StreamDocuments but converts each
// *scippb.Document to *Document before passing it to fn. This lets callers
// in other packages work with the typed Document without importing the proto
// bindings directly.
//
// Only the fields needed for incremental ingestion are converted
// (RelativePath, Language, Occurrences, Symbols). Index structures
// (RefIndex, DefIndex, ContainerIndex, etc.) are never built.
func StreamConvertedDocuments(path string, fn func(*Document) error) error {
	return StreamDocuments(path, func(pbDoc *scippb.Document) error {
		return fn(convertDocument(pbDoc))
	})
}

// StreamDocuments parses the SCIP index at path and calls fn once for each
// document, in file order. It never materialises a full SCIPIndex — only one
// *scippb.Document is live at a time, keeping peak memory proportional to the
// largest single document rather than the whole index.
//
// This is intended for ingestion pipelines (e.g. PopulateFromFullIndex) that
// need to iterate documents once. It is not suitable for random-access queries,
// which require the full in-memory SCIPIndex built by LoadSCIPIndex.
//
// fn must not retain a reference to the document after returning — the
// underlying proto bytes are mmap-owned and may be recycled.
//
// Returns the first error from fn, or a parse error if the file is malformed.
func StreamDocuments(path string, fn func(*scippb.Document) error) error {
	data, cleanup, err := mapFile(path)
	if err != nil {
		return fmt.Errorf("stream scip %s: %w", path, err)
	}
	defer cleanup()

	opts := proto.UnmarshalOptions{DiscardUnknown: true}
	b := data
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return fmt.Errorf("stream scip: invalid tag at offset %d", len(data)-len(b))
		}
		b = b[n:]

		switch num {
		case 1: // Metadata — skip
			_, n := protowire.ConsumeBytes(b)
			if n < 0 {
				b = b[max(n, 1):]
				continue
			}
			b = b[n:]

		case 2: // Document
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				b = b[max(n, 1):]
				continue
			}
			b = b[n:]
			var d scippb.Document
			if opts.Unmarshal(v, &d) != nil {
				continue // skip malformed documents
			}
			if err := fn(&d); err != nil {
				return err
			}

		default: // external_symbols (field 3) or unknown — skip
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				b = b[max(n, 1):]
				continue
			}
			b = b[n:]
		}
	}
	return nil
}
