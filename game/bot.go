package game

import (
	"context"
	"math/rand"
)

// CallingStation checks when it can and calls when it cannot. It is the
// default source for a seat, and the baseline every engine test runs
// against because its behaviour is completely predictable.
type CallingStation struct{}

func (CallingStation) RequestAction(_ context.Context, v PlayerView) (Decision, error) {
	if v.ToCall == 0 {
		return Decision{Action: Check}, nil
	}
	return Decision{Action: Call}, nil
}

// RandomBot picks uniformly among the legal actions. It exists to shake
// the engine hard: all-ins for less than a full raise, folds in spots
// nobody would fold, and every side-pot shape that follows from those.
type RandomBot struct {
	Rand *rand.Rand
}

func (b RandomBot) RequestAction(_ context.Context, v PlayerView) (Decision, error) {
	r := b.Rand
	if r == nil {
		r = rand.New(rand.NewSource(1))
	}

	options := make([]Decision, 0, 4)

	if v.ToCall > 0 {
		options = append(options, Decision{Action: Fold})
		options = append(options, Decision{Action: Call})
	} else {
		options = append(options, Decision{Action: Check})
	}

	if v.Legal(Raise) {
		if v.MinRaiseTo <= v.MaxRaiseTo {
			span := v.MaxRaiseTo - v.MinRaiseTo
			target := v.MinRaiseTo
			if span > 0 {
				target += r.Intn(span + 1)
			}
			options = append(options, Decision{Action: Raise, Amount: target})
		}
		// Shoving is legal even when it falls short of a full raise.
		options = append(options, Decision{Action: Raise, Amount: v.MaxRaiseTo})
	}

	return options[r.Intn(len(options))], nil
}
