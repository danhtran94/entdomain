package pbmap

import (
	"github.com/danhtran94/entdomain/examples/basic/domain"
	"github.com/danhtran94/entdomain/examples/basic/proto/entpb"
)

func UserMetadataToProto(m domain.UserMetadata) *entpb.UserMetadata {
	return &entpb.UserMetadata{Links: m.Links}
}

func UserMetadataFromProto(p *entpb.UserMetadata) domain.UserMetadata {
	if p == nil {
		return domain.UserMetadata{}
	}
	return domain.UserMetadata{Links: p.Links}
}
