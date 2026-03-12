package jsonapi

import "github.com/dhamidi/hyper"

// DefaultCodecs returns a RepresentationCodec and a SubmissionCodec
// ready for registration with a hyper Renderer and Client respectively.
//
//	repCodec, subCodec := jsonapi.DefaultCodecs()
func DefaultCodecs() (hyper.RepresentationCodec, hyper.SubmissionCodec) {
	return RepresentationCodec{}, SubmissionCodec{}
}
