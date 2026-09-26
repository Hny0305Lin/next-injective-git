package s3store

import (
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"testing"
)

func TestAmbiguousWritePreservesIntegrityFailure(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, []byte("expected"))
	key, _ := s.key(o)
	f.objects["/fixture-bucket/"+key] = []byte("badbytes")
	f.putStatuses = []int{503, 503, 503}
	r, e := s.PutIfAbsent(context.Background(), o, src)
	code(t, e, packstore.Integrity)
	if r.Verified || r.Key == "" {
		t.Fatal("invalid uncertain receipt", r)
	}
}
