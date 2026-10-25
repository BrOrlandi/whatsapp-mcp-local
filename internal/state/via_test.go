package state

import (
	"context"
	"testing"
	"time"
)

func TestAClientRemembersItsWay(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_ = st.SeeClientVia(ctx, "hermes-agent", "1", "url", time.Now())
	_ = st.SeeClient(ctx, "hermes-agent", "", time.Now()) // a call that does not say keeps the way
	list, _ := st.Clients(ctx)
	if len(list) != 1 || list[0].Via != "url" {
		t.Fatalf("clients: %+v", list)
	}
}
