package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

// fakeHouse is the house of the tests: it hands over a machine and remembers
// what it was asked, so a turn can be measured without a bill.
type fakeHouse struct {
	addr    string
	state   string
	created []compute.Spec
	keys    []string
	stopped []string
	resumed []string
}

func (f *fakeHouse) Create(_ context.Context, spec compute.Spec) (compute.Sandbox, error) {
	f.created = append(f.created, spec)
	return compute.Sandbox{ID: "bx_de_mentira", State: "provisioning"}, nil
}

func (f *fakeHouse) Wait(_ context.Context, id string) (compute.Sandbox, error) {
	return compute.Sandbox{ID: id, State: "ready", Addr: f.addr, User: "user"}, nil
}

func (f *fakeHouse) Get(_ context.Context, id string) (compute.Sandbox, error) {
	return compute.Sandbox{ID: id, State: f.state, Addr: f.addr, User: "user"}, nil
}

func (f *fakeHouse) Stop(_ context.Context, id string) error {
	f.stopped = append(f.stopped, id)
	return nil
}

func (f *fakeHouse) Resume(_ context.Context, id string) error {
	f.resumed = append(f.resumed, id)
	f.state = "ready"
	return nil
}

func (f *fakeHouse) Delete(_ context.Context, _ string) error { return nil }

func (f *fakeHouse) Authorize(_ context.Context, _ string, key string) error {
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeHouse) Host(_ context.Context, _ string, _ int) (string, error) {
	return "https://de-mentira-3000.on.boat.dev", nil
}

var _ compute.Provider = (*fakeHouse)(nil)

// withoutMachine leaves an owner that came with a machine of their own without
// it, which is where an account that never loaded one starts.
func withoutMachine(t *testing.T, service *app.Service) (chat.Owner, string, auth.User) {
	t.Helper()
	conversation := apptest.Thread(t, service)
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.RemoveWorkspace(t.Context(), owner, "prueba"); err != nil {
		t.Fatalf("sacar la máquina: %v", err)
	}
	return owner, conversation.ID, user
}

func TestATurnWithoutAMachineGetsOneFromTheHouse(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"content":"hola"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
	))
	service.House = &fakeHouse{addr: "127.0.0.1:2222"}
	owner, conversation, user := withoutMachine(t, service)
	house := service.House.(*fakeHouse)

	if err := service.Say(t.Context(), conversation, user, "hola", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation)

	if len(house.created) != 1 {
		t.Fatalf("la casa creó %d máquinas", len(house.created))
	}
	if spec := house.created[0]; spec.Size != app.HouseSize || spec.TTL != app.HouseTTL {
		t.Fatalf("pidió %+v", spec)
	}
	if len(house.keys) != 1 || !strings.HasPrefix(house.keys[0], "ssh-ed25519 ") {
		t.Fatalf("las llaves quedaron %v", house.keys)
	}
	space, err := service.Workspace(t.Context(), owner)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if space.Provider != app.HouseProvider || space.SandboxID != "bx_de_mentira" {
		t.Fatalf("la fila quedó %+v", space)
	}
	sandbox, err := service.Sandbox(t.Context(), owner)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if err := sandbox.Ready(); err != nil {
		t.Fatalf("el sandbox no quedó listo: %v", err)
	}
	if sandbox.Addr != "127.0.0.1:2222" || sandbox.User != "user" || len(sandbox.Key) == 0 {
		t.Fatalf("los secretos quedaron %+v", sandbox)
	}
	if lines := apptest.Events(t, service, conversation); len(lines) < 3 {
		t.Fatalf("el hilo quedó %v", lines)
	}
}

func TestAMachineOfTheHouseThatIsAsleepIsWokenUp(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"content":"uno"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"dos"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
	))
	house := &fakeHouse{addr: "127.0.0.1:2222"}
	service.House = house
	owner, conversation, user := withoutMachine(t, service)

	if err := service.Say(t.Context(), conversation, user, "hola", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation)

	house.state = "archived"
	if err := service.Say(t.Context(), conversation, user, "otra vez", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation)

	if len(house.created) != 1 {
		t.Fatalf("la casa creó %d máquinas: tendría que haber reusado la suya", len(house.created))
	}
	if len(house.resumed) != 1 {
		t.Fatalf("despertó %d veces", len(house.resumed))
	}
	space, err := service.Workspace(t.Context(), owner)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if space.SandboxID != "bx_de_mentira" {
		t.Fatalf("la fila quedó %+v", space)
	}
}

func TestAMachineOfTheOwnerIsNotTheBusinessOfTheHouse(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"content":"hola"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
	))
	house := &fakeHouse{addr: "127.0.0.1:2222"}
	service.House = house
	conversation := apptest.Thread(t, service)
	user := apptest.User(t, service)

	if err := service.Say(t.Context(), conversation.ID, user, "hola", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	if len(house.created) != 0 || len(house.resumed) != 0 {
		t.Fatalf("la casa hizo algo con una máquina ajena: %+v %+v", house.created, house.resumed)
	}
}
