package utils

import (
	"crypto/sha1"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// A fake implementation of the getImage function that does not query remote container registries.
var FakeGetRemoteImageFunc = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
	h := sha1.New()
	h.Write([]byte(ref.Identifier()))

	return &remote.Descriptor{
		Descriptor: v1.Descriptor{
			Digest: v1.Hash{
				Algorithm: "sha256",
				Hex:       fmt.Sprintf("%x\n", h.Sum(nil)),
			},
		},
	}, nil
}
