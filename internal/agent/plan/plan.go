// Package plan provides a shared execution-plan model and operation-level
// gating used by the agent engines.
package plan

// TodoStatus is the lifecycle state of a plan todo.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoDone       TodoStatus = "done"
	TodoSkipped    TodoStatus = "skipped"
)

// Todo is one tracked unit of work within a Plan.
type Todo struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Stage  string     `json:"stage,omitempty"`
	Status TodoStatus `json:"status"`
}

// Plan is an ordered set of todos with a goal, supporting coverage tracking.
type Plan struct {
	Goal    string `json:"goal"`
	Todos   []Todo `json:"todos"`
	Version int    `json:"version"`
}

// Coverage returns how many todos are resolved (done or skipped) and the total.
func (p *Plan) Coverage() (resolved, total int) {
	total = len(p.Todos)
	for _, t := range p.Todos {
		if t.Status == TodoDone || t.Status == TodoSkipped {
			resolved++
		}
	}
	return resolved, total
}

// NextPending returns the first pending todo, or nil if none remain.
func (p *Plan) NextPending() *Todo {
	for i := range p.Todos {
		if p.Todos[i].Status == TodoPending {
			return &p.Todos[i]
		}
	}
	return nil
}

// Mark sets the status of the todo with the given id. Returns false if absent.
func (p *Plan) Mark(id string, status TodoStatus) bool {
	for i := range p.Todos {
		if p.Todos[i].ID == id {
			p.Todos[i].Status = status
			return true
		}
	}
	return false
}
