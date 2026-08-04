package deck

type Player struct {
	hand Hand
}

func newPlayer(d *Deck) Player {
	var player = Player{
		hand: Hand{},
	}

}
