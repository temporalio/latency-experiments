package signalquery

import (
	"context"
	"time"

	. "github.com/dandavison/temporal-latency-experiments/must"
	"github.com/dandavison/temporal-latency-experiments/tle"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"
)

const (
	SignalName     = "my-signal"
	QueryName      = "my-query"
	DoneSignalName = "Done"
	WorkflowID     = "my-workflow-id"
)

// Send a signal and immediately send a query and assert that it read the
// signal's writes to local workflow state.
func Run(c client.Client, l sdklog.Logger, iterations int) tle.Results {
	ctx := context.Background()
	Must(c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                    WorkflowID,
		TaskQueue:             tle.TaskQueue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_TERMINATE_IF_RUNNING,
	}, MyWorkflow))

	latencies := []int64{}
	wfts := []int{}
	for i := 0; i < iterations; i++ {
		start := time.Now()

		go Must1(c.SignalWorkflow(ctx, WorkflowID, "", SignalName, i))

		queryResult := Must(c.QueryWorkflow(ctx, WorkflowID, "", QueryName))
		var result int
		Must1(queryResult.Get(&result))
		if result != i+1 {
			panic("query did not read signal's write")
		}

		latency := time.Since(start).Nanoseconds()
		latencies = append(latencies, latency)
		wfts = append(wfts, tle.CountWorkflowTasks(c, WorkflowID, ""))
	}
	Must1(c.SignalWorkflow(ctx, WorkflowID, "", DoneSignalName, nil))

	return tle.Results{
		LatenciesNs: latencies,
		Wfts:        wfts,
	}
}

func MyWorkflow(ctx workflow.Context) (int, error) {
	counter := 0

	workflow.SetQueryHandler(ctx, QueryName, func() (int, error) {
		return counter, nil
	})

	ch := workflow.GetSignalChannel(ctx, SignalName)

	sel := workflow.NewSelector(ctx)
	sel.AddReceive(ch, func(c workflow.ReceiveChannel, more bool) {
		var signal int
		c.Receive(ctx, &signal)
		counter++
	})

	doneCh := workflow.GetSignalChannel(ctx, DoneSignalName)

	for {
		sel.Select(ctx)
		if doneCh.ReceiveAsync(nil) {
			break
		}
	}

	return counter, nil
}
