package main

import (
	"errors"
	"fmt"
	"sync"
)

// Every scenario owns a separate loopback mock and private product HOME.
// Otherwise one scenario's rotation or error plan could contaminate another
// under concurrent execution, silently turning a parity failure into PASS.
type differentialScenario043713 struct {
	name string
	run  func(*mockServer) (scenarioResult, error)
}

// executeDifferentialScenarios043713 executes the same checked scenarios in
// declaration order. Only execution is parallel; output and error decisions
// remain deterministic. It never returns partial successful evidence if any
// scenario or mock startup fails.
func executeDifferentialScenarios043713(
	specs []differentialScenario043713, scenarioName string, workers int,
) ([]scenarioResult, error) {
	if workers < 1 || workers > 4 {
		return nil, errors.New("scenario workers must be in [1,4]")
	}
	if workers > 1 && scenarioName != "all" {
		return nil, errors.New("parallel scenario workers require --scenario all")
	}
	selected := make([]differentialScenario043713, 0, len(specs))
	for _, spec := range specs {
		if scenarioName == "all" || scenarioName == spec.name {
			selected = append(selected, spec)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("unknown scenario %q", scenarioName)
	}
	results := make([]scenarioResult, len(selected))
	problems := make([]error, len(selected))
	jobs := make(chan int)
	var group sync.WaitGroup
	for range min(workers, len(selected)) {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				spec := selected[index]
				mock, err := startMock()
				if err != nil {
					problems[index] = err
					continue
				}
				result, err := spec.run(mock)
				mock.close()
				results[index] = result
				problems[index] = err
			}
		}()
	}
	for index := range selected {
		jobs <- index
	}
	close(jobs)
	group.Wait()
	for index, err := range problems {
		if err != nil {
			return nil, fmt.Errorf("%s scenario: %w", selected[index].name, err)
		}
	}
	return results, nil
}
