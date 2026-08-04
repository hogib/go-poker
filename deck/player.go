package deck

type Player struct {
	Hand Hand
}

func NewPlayer(d *Deck) (Player, error) {
	startingHand, err := DealHand(d)
	if err != nil {
		return Player{}, err
	}

	return Player{
		Hand: startingHand,
	}, nil
}
