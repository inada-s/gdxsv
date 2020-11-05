package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const rateLimit = time.Second

var discordRequestGroup singleflight.Group

func (lbs *Lbs) PublishStatusToDiscord() {
	if len(conf.DiscordWebhookURL) == 0 {
		return
	}

	type onlineUser struct {
		UserID     string `json:"user_id,omitempty"`
		Name       string `json:"name,omitempty"`
		Team       string `json:"team,omitempty"`
		BattleCode string `json:"battle_code,omitempty"`
		Disk       string `json:"disk,omitempty"`
	}

	type DiscordEmbedFooter struct {
		Text string `json:"text,omitempty"`
	}

	type DiscordEmbedField struct {
		Name   string `json:"name,omitempty"`
		Value  string `json:"value,omitempty"`
		Inline bool   `json:"inline,omitempty"`
	}

	type DiscordEmbed struct {
		Title       string               `json:"title,omitempty"`
		Description string               `json:"description,omitempty"`
		Color       int                  `json:"color,omitempty"`
		Footer      *DiscordEmbedFooter  `json:"footer,omitempty"`
		Timestamp   string               `json:"timestamp,omitempty"`
		Fields      []*DiscordEmbedField `json:"fields,omitempty"`
	}

	type statusPayload struct {
		Embed   []*DiscordEmbed `json:"embeds"`
		BotName string          `json:"username"`
	}

	type LobbyPeers struct {
		Count          int
		Name           string
		RegionName     string
		Comment        string
		RenpoPeers     string
		ZeonPeers      string
		RenpoRoomPeers string
		ZeonRoomPeers  string
		NoForcePeers   string
	}

	type BattlePeers struct {
		RegionName string
		RenpoPeers string
		ZeonPeers  string
	}

	contains := func(s []string, e string) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	sortedKeys := func(m map[uint16]*LobbyPeers) []uint16 {
		keys := make([]uint16, len(m))
		i := 0
		for k := range m {
			keys[i] = k
			i++
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		return keys
	}

	//insert Braille Pattern Blank to create newline at ending
	addBlankIfRequired := func(s string) string {
		if len(s) > 0 {
			return s + "⠀\n"
		}
		return s
	}

	//Embed limits: Field's value is limited toto 1024 characters
	splitFieldValues := func(value string) []string {
		var values []string

		//Split field values into 1024 chunks
		numberOfSplit := int(math.Ceil(float64(len(value)) / 1024))
		fmt.Println(numberOfSplit)

		splitIndex := 0
		for i := 0; i < numberOfSplit; i++ {
			splitIndex = len(value) / (numberOfSplit - i)
			firstHalf := value[:splitIndex]
			splitIndex = strings.LastIndex(firstHalf, "\n")

			values = append(values, value[:splitIndex])
			// splitIndex += 2
			value = value[splitIndex:]
		}
		//do not create empty fields
		if value != "\n" {
			values = append(values, value)
		}

		return values
	}

	//Embed limits: There can be up to 25 fields
	splitFields := func(fields []*DiscordEmbedField) [][]*DiscordEmbedField {
		var divided [][]*DiscordEmbedField
		chunkSize := 25

		for i := 0; i < len(fields); i += chunkSize {
			end := i + chunkSize

			if end > len(fields) {
				end = len(fields)
			}

			divided = append(divided, fields[i:end])
		}
		return divided
	}

	reduceStringSize := func(s string) string {
		//Reduce size by removing userid
		re := regexp.MustCompile("`.*?`\\s")
		s = re.ReplaceAllString(s, "")

		//Reduce size by replacing custom emoji (from 28 char to 1 char)
		s = strings.ReplaceAll(s, "<:gundam:772467554160738355>", "🌎")
		s = strings.ReplaceAll(s, "<:zaku:772467605008023563>", "🪐")

		return s
	}

	payload, _, _ := discordRequestGroup.Do("discord_webhook_publish", func() (interface{}, error) {

		battle := make(map[string]*BattlePeers)

		battlePeerCount := 0
		var battlePeersIDs string
		var accumulatedBattlePeersString string

		for _, u := range sharedData.GetMcsUsers() {
			_, exists := battle[u.BattleCode]
			if exists == false {
				battle[u.BattleCode] = new(BattlePeers)

				locName, ok := gcpLocationName[u.McsRegion]
				if !ok {
					locName = "Default Server"
				}
				if u.McsRegion == "best" {
					locName = "Best Server"
				}
				battle[u.BattleCode].RegionName = locName
				accumulatedBattlePeersString += locName
			}

			var peer string
			switch u.Side {
			case TeamRenpo:
				peer = fmt.Sprintf("<:gundam:772467554160738355> `%s` %s\n", u.UserID, u.Name)
				battle[u.BattleCode].RenpoPeers += peer
			case TeamZeon:
				peer = fmt.Sprintf("<:zaku:772467605008023563> `%s` %s\n", u.UserID, u.Name)
				battle[u.BattleCode].ZeonPeers += peer
			}
			accumulatedBattlePeersString += peer

			battlePeerCount++
			battlePeersIDs += u.UserID
		}

		var plazaPeers string

		lobby := make(map[uint16]*LobbyPeers)

		plazaPeerCount := 0
		lobbyPeerCount := 0

		var accumulatedLobbyPeersString string

		for _, u := range lbs.userPeers {

			//Already in battle, hidden from lobby
			if strings.Contains(battlePeersIDs, u.UserID) {
				continue
			}

			if u.Lobby == nil {
				plazaPeers += fmt.Sprintf("`%s` %s\n", u.UserID, u.Name)
				accumulatedLobbyPeersString += plazaPeers
				plazaPeerCount++
			} else {
				_, exists := lobby[u.Lobby.ID]
				if exists == false {
					lobby[u.Lobby.ID] = new(LobbyPeers)
					lobby[u.Lobby.ID].Name = u.Lobby.Name

					locName, ok := gcpLocationName[u.Lobby.McsRegion]
					if !ok {
						locName = "Default Server"
					}
					if u.Lobby.McsRegion == "best" {
						locName = "Best Server"
					}
					lobby[u.Lobby.ID].RegionName = locName

					var comment string
					if strings.Contains(u.Lobby.Comment, "TeamShuffle") {
						comment += "🔀"
					}
					if strings.Contains(u.Lobby.Comment, "For JP vs HK") {
						comment += "( 🇯🇵 vs 🇭🇰 )"
					}
					if strings.Contains(u.Lobby.Comment, "Private Room") {
						comment += "🔒"
					}
					if strings.Contains(u.Lobby.Comment, "No 375 Cost MS") {
						comment += "⛔375"
					}
					if strings.Contains(u.Lobby.Comment, "3R") {
						comment += "3️⃣"
					}

					lobby[u.Lobby.ID].Comment = comment
				}

				var readyColor string
				if contains(u.Lobby.EntryUsers, u.UserID) {
					readyColor = "🟢"
				} else {
					readyColor = "🔴"
				}
				if u.Room != nil {
					readyColor = "📢"
				}
				var peer string
				switch u.Team {
				case TeamRenpo:
					peer = fmt.Sprintf("<:gundam:772467554160738355>%s `%s` %s\n", readyColor, u.UserID, u.Name)
					if u.Room == nil {
						lobby[u.Lobby.ID].RenpoPeers += peer
					} else {
						lobby[u.Lobby.ID].RenpoRoomPeers += peer
					}
				case TeamZeon:
					peer = fmt.Sprintf("<:zaku:772467605008023563>%s `%s` %s\n", readyColor, u.UserID, u.Name)
					if u.Room == nil {
						lobby[u.Lobby.ID].ZeonPeers += peer
					} else {
						lobby[u.Lobby.ID].ZeonRoomPeers += peer
					}
				default:
					peer = fmt.Sprintf("❔⚫ `%s` %s\n", u.UserID, u.Name)
					lobby[u.Lobby.ID].NoForcePeers += peer
				}
				accumulatedLobbyPeersString += peer

				lobby[u.Lobby.ID].Count++
			}
			lobbyPeerCount++
		}

		embedAccumulatedLobbyStringLength := len(accumulatedLobbyPeersString)
		println("Lobby Embed length: ", embedAccumulatedLobbyStringLength)
		if embedAccumulatedLobbyStringLength > 6000 {
			plazaPeers = reduceStringSize(plazaPeers)

			for _, l := range lobby {
				l.RenpoPeers = reduceStringSize(l.RenpoPeers)
				l.ZeonPeers = reduceStringSize(l.ZeonPeers)
				l.RenpoRoomPeers = reduceStringSize(l.RenpoRoomPeers)
				l.ZeonRoomPeers = reduceStringSize(l.ZeonRoomPeers)
				l.NoForcePeers = reduceStringSize(l.NoForcePeers)
			}
		}
		embedAccumulatedBattleStringLength := len(accumulatedBattlePeersString)
		println("Battle Embed length: ", embedAccumulatedBattleStringLength)
		if embedAccumulatedBattleStringLength > 6000 {
			for _, b := range battle {
				b.RenpoPeers = reduceStringSize(b.RenpoPeers)
				b.ZeonPeers = reduceStringSize(b.ZeonPeers)
			}
		}

		payload := new(statusPayload)
		payload.BotName = "Live Status"

		//1st Embed, online count
		payload.Embed = append(payload.Embed, &DiscordEmbed{
			Title:     fmt.Sprintf("**同時接続数 %d 人 **", lobbyPeerCount+battlePeerCount),
			Color:     52224,
			Footer:    &DiscordEmbedFooter{Text: "🕒"},
			Timestamp: fmt.Sprintf(time.Now().UTC().Format("2006-01-02T15:04:05.000Z")),
		})

		//2nd Embed, lobby count
		var lobbyFields []*DiscordEmbedField
		if plazaPeerCount > 0 {
			for i, v := range splitFieldValues(plazaPeers) {
				name := "⠀"
				if i == 0 {
					name = fmt.Sprintf("**Plaza － %d 人**", plazaPeerCount)
				}

				lobbyFields = append(lobbyFields, &DiscordEmbedField{
					Name:  name,
					Value: v,
				})
			}
		}

		for _, i := range sortedKeys(lobby) {
			l := lobby[i]

			value := addBlankIfRequired(l.RenpoPeers+l.ZeonPeers) + addBlankIfRequired(l.RenpoRoomPeers+l.ZeonRoomPeers) + addBlankIfRequired(l.NoForcePeers)

			for i, v := range splitFieldValues(value) {
				name := "⠀"
				if i == 0 {
					name = fmt.Sprintf("**%s － %d 人\n%s %s**", l.Name, l.Count, l.RegionName, l.Comment)
				}

				lobbyFields = append(lobbyFields, &DiscordEmbedField{
					Name:  name,
					Value: v,
				})
			}

		}

		for i, fields := range splitFields(lobbyFields) {
			description := ""
			if i == 0 {
				description = fmt.Sprintf("🌐 **待機中 %d 人**", lobbyPeerCount)
			}
			payload.Embed = append(payload.Embed, &DiscordEmbed{
				Description: description,
				Color:       24041,
				Fields:      fields,
			})
		}

		//3rd Embed, battle count
		var battleFields []*DiscordEmbedField
		for _, b := range battle {

			value := addBlankIfRequired(b.RenpoPeers + b.ZeonPeers)

			for i, v := range splitFieldValues(value) {
				name := "⠀"
				if i == 0 {
					name = b.RegionName
				}

				battleFields = append(battleFields, &DiscordEmbedField{
					Name:  name,
					Value: v,
				})
			}
		}

		for i, fields := range splitFields(battleFields) {
			description := ""
			if i == 0 {
				description = fmt.Sprintf("💥 **戦闘中 %d 人**", battlePeerCount)
			}
			payload.Embed = append(payload.Embed, &DiscordEmbed{
				Description: description,
				Color:       13179394,
				Fields:      fields,
			})
		}

		return payload, nil
	})

	var jsonData []byte
	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to create Discord JSON", zap.Error(err))
		return
	}
	logger.Info(string(jsonData))

	req, err := http.NewRequest("PATCH", conf.DiscordWebhookURL, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to publish to Discord", zap.Error(err))
	}
	defer resp.Body.Close()
	logger.Info("Discord Webhook done", zap.String("Status", resp.Status))
}
