package heimdall

import "fmt"

type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

const (
	OwnerOrg  = "org"
	OwnerUser = "user"
)

func Org(id string) Owner {
	return Owner{Kind: OwnerOrg, ID: id}
}

func User(id string) Owner {
	return Owner{Kind: OwnerUser, ID: id}
}

func (o Owner) String() string {
	return o.Kind + "/" + o.ID
}

func (o Owner) check() error {
	if o.Kind != OwnerOrg && o.Kind != OwnerUser {
		return bad(fmt.Sprintf("«%s» no es una organización ni un usuario", o.Kind))
	}
	return slug(o.ID)
}
