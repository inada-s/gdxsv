package lobby

type symbol struct {
	No       byte
	Dir      byte
	Category byte
	Command  uint16
	Address  uint32
	Name     string
}

var symbolMap map[uint16]*symbol

func init() {
	symbolMap = map[uint16]*symbol{}
	for i, s := range symbolList {
		symbolMap[s.Command] = &symbolList[i]
	}
}

var symbolList = []symbol{}
