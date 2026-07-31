package hermescmd

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.temporal.io/sdk/client"
)

type fakeScheduleCleanupClient struct {
	lists       []fakeScheduleList
	listCalls   int
	deleted     []string
	deleteError map[string]error
}

type fakeScheduleList struct {
	pages   [][]string
	listErr error
	nextErr error
}

func (c *fakeScheduleCleanupClient) List(
	context.Context,
) (scheduleCleanupIterator, error) {
	if c.listCalls >= len(c.lists) {
		return nil, errors.New("unexpected list")
	}
	list := c.lists[c.listCalls]
	c.listCalls++
	if list.listErr != nil {
		return nil, list.listErr
	}
	return &fakeScheduleCleanupIterator{
		pages:   list.pages,
		nextErr: list.nextErr,
	}, nil
}

func (c *fakeScheduleCleanupClient) Delete(_ context.Context, id string) error {
	c.deleted = append(c.deleted, id)
	return c.deleteError[id]
}

type fakeScheduleCleanupIterator struct {
	pages     [][]string
	page      int
	entry     int
	nextErr   error
	errRaised bool
}

func (i *fakeScheduleCleanupIterator) HasNext() bool {
	for i.page < len(i.pages) && i.entry >= len(i.pages[i.page]) {
		i.page++
		i.entry = 0
	}
	return i.page < len(i.pages) || (i.nextErr != nil && !i.errRaised)
}

func (i *fakeScheduleCleanupIterator) Next() (string, error) {
	if i.page >= len(i.pages) {
		i.errRaised = true
		return "", i.nextErr
	}
	id := i.pages[i.page][i.entry]
	i.entry++
	return id, nil
}

func TestCleanupOpaqueSettlementSchedulesCommandUsesConfiguredTemporalClient(
	t *testing.T,
) {
	t.Setenv("TEMPORAL_ADDRESS", "temporal.example:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "operator-namespace")
	fake := &fakeScheduleCleanupClient{lists: []fakeScheduleList{
		{pages: [][]string{{"opaque-settlement-discovery:one"}}},
		{pages: [][]string{{"unrelated"}}},
	}}
	closed := false
	var gotOptions client.Options
	command := newCleanupOpaqueSettlementSchedulesCommand(
		func(options client.Options) (scheduleCleanupClient, func(), error) {
			gotOptions = options
			return fake, func() { closed = true }, nil
		},
	)
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotOptions.HostPort != "temporal.example:7233" ||
		gotOptions.Namespace != "operator-namespace" {
		t.Fatalf("Temporal options = %#v", gotOptions)
	}
	if !closed {
		t.Fatal("Temporal client was not closed")
	}
	if got := output.String(); !strings.Contains(got, "verification found 0 remaining") {
		t.Fatalf("output = %q", got)
	}
}

func TestCleanupOpaqueSettlementSchedulesPaginatesDeletesAndRelists(t *testing.T) {
	t.Parallel()

	client := &fakeScheduleCleanupClient{lists: []fakeScheduleList{
		{pages: [][]string{
			{"unrelated", "opaque-settlement-discovery:first"},
			{
				"prefix-opaque-settlement-discovery:lookalike",
				"opaque-settlement-discovery:second",
			},
		}},
		{pages: [][]string{{"unrelated"}, {"another"}}},
	}}

	deleted, err := cleanupOpaqueSettlementSchedules(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"opaque-settlement-discovery:first",
		"opaque-settlement-discovery:second",
	}
	if !reflect.DeepEqual(deleted, want) || !reflect.DeepEqual(client.deleted, want) {
		t.Fatalf("deleted = %v, calls = %v; want %v", deleted, client.deleted, want)
	}
	if client.listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", client.listCalls)
	}
}

func TestCleanupOpaqueSettlementSchedulesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		client     *fakeScheduleCleanupClient
		wantError  string
		wantDelete []string
	}{
		{
			name: "initial list",
			client: &fakeScheduleCleanupClient{
				lists: []fakeScheduleList{{listErr: errors.New("list failed")}},
			},
			wantError: "list retired opaque settlement schedules",
		},
		{
			name: "initial pagination",
			client: &fakeScheduleCleanupClient{lists: []fakeScheduleList{{
				pages: [][]string{{"unrelated"}}, nextErr: errors.New("page failed"),
			}}},
			wantError: "list retired opaque settlement schedules",
		},
		{
			name: "delete",
			client: &fakeScheduleCleanupClient{
				lists: []fakeScheduleList{
					{pages: [][]string{{"opaque-settlement-discovery:one"}}},
				},
				deleteError: map[string]error{
					"opaque-settlement-discovery:one": errors.New("delete failed"),
				},
			},
			wantError:  "delete retired schedule",
			wantDelete: []string{"opaque-settlement-discovery:one"},
		},
		{
			name: "relist",
			client: &fakeScheduleCleanupClient{lists: []fakeScheduleList{
				{pages: [][]string{{"opaque-settlement-discovery:one"}}},
				{listErr: errors.New("relist failed")},
			}},
			wantError:  "verify retired opaque settlement schedules",
			wantDelete: []string{"opaque-settlement-discovery:one"},
		},
		{
			name: "remaining",
			client: &fakeScheduleCleanupClient{lists: []fakeScheduleList{
				{pages: [][]string{{"opaque-settlement-discovery:one"}}},
				{pages: [][]string{{"opaque-settlement-discovery:one"}}},
			}},
			wantError:  "retired opaque settlement schedules remain: opaque-settlement-discovery:one",
			wantDelete: []string{"opaque-settlement-discovery:one"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := cleanupOpaqueSettlementSchedules(t.Context(), test.client)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want containing %q", err, test.wantError)
			}
			if !reflect.DeepEqual(test.client.deleted, test.wantDelete) {
				t.Fatalf("deleted = %v, want %v", test.client.deleted, test.wantDelete)
			}
		})
	}
}
