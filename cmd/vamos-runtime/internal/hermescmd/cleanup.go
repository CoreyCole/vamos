package hermescmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
)

const opaqueSettlementSchedulePrefix = "opaque-settlement-discovery:"

type scheduleCleanupIterator interface {
	HasNext() bool
	Next() (string, error)
}

type scheduleCleanupClient interface {
	List(context.Context) (scheduleCleanupIterator, error)
	Delete(context.Context, string) error
}

type temporalScheduleCleanupClient struct {
	client client.ScheduleClient
}

func (c temporalScheduleCleanupClient) List(
	ctx context.Context,
) (scheduleCleanupIterator, error) {
	iterator, err := c.client.List(ctx, client.ScheduleListOptions{})
	if err != nil {
		return nil, err
	}
	return temporalScheduleCleanupIterator{iterator: iterator}, nil
}

func (c temporalScheduleCleanupClient) Delete(ctx context.Context, id string) error {
	return c.client.GetHandle(ctx, id).Delete(ctx)
}

type temporalScheduleCleanupIterator struct {
	iterator client.ScheduleListIterator
}

func (i temporalScheduleCleanupIterator) HasNext() bool {
	return i.iterator.HasNext()
}

func (i temporalScheduleCleanupIterator) Next() (string, error) {
	entry, err := i.iterator.Next()
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

type cleanupTemporalDialer func(
	client.Options,
) (scheduleCleanupClient, func(), error)

func defaultCleanupTemporalDialer(
	options client.Options,
) (scheduleCleanupClient, func(), error) {
	connection, err := client.Dial(options)
	if err != nil {
		return nil, nil, err
	}
	return temporalScheduleCleanupClient{client: connection.ScheduleClient()},
		connection.Close, nil
}

func newCleanupOpaqueSettlementSchedulesCommand(
	dial cleanupTemporalDialer,
) *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup-opaque-settlement-schedules",
		Short: "Delete retired opaque settlement discovery schedules",
		RunE: func(cmd *cobra.Command, _ []string) error {
			address := strings.TrimSpace(os.Getenv("TEMPORAL_ADDRESS"))
			if address == "" {
				address = "localhost:7233"
			}
			options := client.Options{HostPort: address}
			if namespace := strings.TrimSpace(
				os.Getenv("TEMPORAL_NAMESPACE"),
			); namespace != "" {
				options.Namespace = namespace
			}
			cleanupClient, closeClient, err := dial(options)
			if err != nil {
				return fmt.Errorf("connect to Temporal: %w", err)
			}
			defer closeClient()

			deleted, err := cleanupOpaqueSettlementSchedules(
				cmd.Context(),
				cleanupClient,
			)
			if err != nil {
				return err
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"deleted %d retired opaque settlement schedule(s); verification found 0 remaining\n",
				len(deleted),
			)
			return nil
		},
	}
}

func cleanupOpaqueSettlementSchedules(
	ctx context.Context,
	client scheduleCleanupClient,
) ([]string, error) {
	matching, err := listOpaqueSettlementSchedules(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("list retired opaque settlement schedules: %w", err)
	}
	for _, id := range matching {
		if err := client.Delete(ctx, id); err != nil {
			return matching, fmt.Errorf("delete retired schedule %q: %w", id, err)
		}
	}
	remaining, err := listOpaqueSettlementSchedules(ctx, client)
	if err != nil {
		return matching, fmt.Errorf("verify retired opaque settlement schedules: %w", err)
	}
	if len(remaining) != 0 {
		return matching, fmt.Errorf(
			"retired opaque settlement schedules remain: %s",
			strings.Join(remaining, ", "),
		)
	}
	return matching, nil
}

func listOpaqueSettlementSchedules(
	ctx context.Context,
	client scheduleCleanupClient,
) ([]string, error) {
	iterator, err := client.List(ctx)
	if err != nil {
		return nil, err
	}
	var matching []string
	for iterator.HasNext() {
		id, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(id, opaqueSettlementSchedulePrefix) {
			matching = append(matching, id)
		}
	}
	sort.Strings(matching)
	return matching, nil
}
