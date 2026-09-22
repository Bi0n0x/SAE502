package games

type StatefulGame interface {
	Game

	Start(bet Bet) (state map[string]any, view map[string]any, err error)
	Act(state map[string]any, action string) (next map[string]any, view map[string]any, done bool, outcome Outcome, err error)
	Actions(state map[string]any) []string
}
