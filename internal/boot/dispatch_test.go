package boot_test

import (
	"context"
	"slices"
	"testing"

	"altalune.id/template/internal/boot"
	"altalune.id/template/internal/platform/outbox"
)

type noopDeliverer struct{}

func (noopDeliverer) Deliver(context.Context, outbox.Entry) error { return nil }

func TestBootServer_OutboxIsOnTheKernel(t *testing.T) {
	srv, err := boot.BootServer(context.Background(), newSmokeCfg(t), boot.WithScheduler(false))
	if err != nil {
		t.Fatalf("BootServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if srv.Platform.Outbox == nil {
		t.Fatal("Kernel.Outbox is nil; the durable outbox is unreachable from the composition root")
	}
}

func TestBootServer_DispatchWorkerFollowsTheDeliverer(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []boot.Option
		want bool
	}{
		{name: "no-deliverer", opts: []boot.Option{boot.WithScheduler(false)}},
		{
			name: "deliverer",
			opts: []boot.Option{boot.WithScheduler(false), boot.WithDispatch(noopDeliverer{}, outbox.WorkerOpts{})},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := boot.BootServer(context.Background(), newSmokeCfg(t), tc.opts...)
			if err != nil {
				t.Fatalf("BootServer: %v", err)
			}
			t.Cleanup(func() { _ = srv.Close() })

			var names []string
			for _, w := range srv.Supervisor.Workers() {
				names = append(names, w.Name())
			}
			if got := slices.Contains(names, outbox.WorkerName); got != tc.want {
				t.Fatalf("supervisor workers = %v, want %q registered = %v", names, outbox.WorkerName, tc.want)
			}
		})
	}
}
