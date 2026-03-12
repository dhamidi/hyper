package hyper

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Renderer selects a RepresentationCodec based on the request's Accept header
// and writes an encoded Representation to the HTTP response.
type Renderer struct {
	Codecs   []RepresentationCodec
	Resolver Resolver
}

// Respond inspects the Accept header, chooses the best codec via content
// negotiation, encodes rep using RenderDocument mode, and writes the response.
func (r Renderer) Respond(w http.ResponseWriter, req *http.Request, status int, rep Representation) error {
	return r.RespondWithMode(w, req, status, rep, RenderDocument)
}

// RespondAs bypasses content negotiation and uses the codec matching mediaType.
func (r Renderer) RespondAs(w http.ResponseWriter, req *http.Request, status int, mediaType string, rep Representation) error {
	codec := r.findCodec(mediaType)
	if codec == nil {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return fmt.Errorf("no codec for media type %q", mediaType)
	}
	return r.writeResponse(w, req, status, rep, codec, mediaType, RenderDocument)
}

// RespondWithMode behaves like Respond but passes the given RenderMode to the codec.
func (r Renderer) RespondWithMode(w http.ResponseWriter, req *http.Request, status int, rep Representation, mode RenderMode) error {
	codec, mediaType := r.negotiate(req)
	if codec == nil {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return fmt.Errorf("no acceptable codec found")
	}
	return r.writeResponse(w, req, status, rep, codec, mediaType, mode)
}

func (r Renderer) writeResponse(w http.ResponseWriter, req *http.Request, status int, rep Representation, codec RepresentationCodec, mediaType string, mode RenderMode) error {
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(status)
	return codec.Encode(req.Context(), w, rep, EncodeOptions{
		Request:  req,
		Resolver: r.Resolver,
		Mode:     mode,
	})
}

// findCodec returns the first codec that supports the given media type.
func (r Renderer) findCodec(mediaType string) RepresentationCodec {
	for _, c := range r.Codecs {
		for _, mt := range c.MediaTypes() {
			if mt == mediaType {
				return c
			}
		}
	}
	return nil
}

// negotiate parses the Accept header and returns the best matching codec and media type.
func (r Renderer) negotiate(req *http.Request) (RepresentationCodec, string) {
	accept := req.Header.Get("Accept")
	if accept == "" {
		if len(r.Codecs) > 0 {
			return r.Codecs[0], r.Codecs[0].MediaTypes()[0]
		}
		return nil, ""
	}

	ranked := parseAccept(accept)

	for _, entry := range ranked {
		for _, c := range r.Codecs {
			for _, mt := range c.MediaTypes() {
				if entry.mediaType == "*/*" || entry.mediaType == mt {
					return c, mt
				}
			}
		}
	}

	// No match — fall back to first codec if available.
	if len(r.Codecs) > 0 {
		return r.Codecs[0], r.Codecs[0].MediaTypes()[0]
	}
	return nil, ""
}

type acceptEntry struct {
	mediaType string
	quality   float64
}

// parseAccept parses an Accept header value into a sorted list of media types.
func parseAccept(header string) []acceptEntry {
	parts := strings.Split(header, ",")
	entries := make([]acceptEntry, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		mt, q := parseMediaRange(p)
		entries = append(entries, acceptEntry{mediaType: mt, quality: q})
	}
	// Sort by quality descending (stable to preserve original order for ties).
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].quality > entries[j-1].quality; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	return entries
}

func parseMediaRange(s string) (string, float64) {
	parts := strings.SplitN(s, ";", 2)
	mt := strings.TrimSpace(parts[0])
	q := 1.0
	if len(parts) > 1 {
		params := strings.TrimSpace(parts[1])
		for _, param := range strings.Split(params, ";") {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") {
				if v, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
					q = v
				}
			}
		}
	}
	return mt, q
}
