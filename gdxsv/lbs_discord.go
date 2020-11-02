package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

	contains := func(s []string, e string) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	//insert Braille Pattern Blank to create newline at ending
	addBlankIfRequired := func(s string) string {
		if len(s) > 0 {
			return s + "⠀\n"
		}
		return s
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

	payload, _, _ := discordRequestGroup.Do("discord_webhook_publish", func() (interface{}, error) {
		type BattlePeers struct {
			RegionName string
			RenpoPeers string
			ZeonPeers  string
		}
		battle := make(map[string]*BattlePeers)

		battlePeerCount := 0
		var battlePeersIDs string
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
			}

			switch u.Side {
			case TeamRenpo:
				battle[u.BattleCode].RenpoPeers += fmt.Sprintf("<:gundam:772467554160738355> `%s` %s\n", u.UserID, u.Name)
			case TeamZeon:
				battle[u.BattleCode].ZeonPeers += fmt.Sprintf("<:zaku:772467605008023563> `%s` %s\n", u.UserID, u.Name)
			}

			battlePeerCount++
			battlePeersIDs += u.UserID
		}
		var plazaPeers string
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
		lobby := make(map[uint16]*LobbyPeers)

		plazaPeerCount := 0
		lobbyPeerCount := 0
		for _, u := range lbs.userPeers {

			//Already in battle, hidden from lobby
			if strings.Contains(battlePeersIDs, u.UserID) {
				continue
			}

			if u.Lobby == nil {
				plazaPeers += fmt.Sprintf("`%s` %s\n", u.UserID, u.Name)
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
					lobby[u.Lobby.ID].NoForcePeers += fmt.Sprintf(":grey_question::black_circle: `%s` %s\n", u.UserID, u.Name)
				}

				lobby[u.Lobby.ID].Count++
			}
			lobbyPeerCount++
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
			lobbyFields = append(lobbyFields, &DiscordEmbedField{
				Name:  fmt.Sprintf("**Plaza － %d 人**", plazaPeerCount),
				Value: plazaPeers + "⠀", //insert Braille Pattern Blank to create newline at ending
			})
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

		for _, i := range sortedKeys(lobby) {
			l := lobby[i]
			lobbyFields = append(lobbyFields, &DiscordEmbedField{
				Name:  fmt.Sprintf("**%s － %d 人\n%s %s**", l.Name, l.Count, l.RegionName, l.Comment),
				Value: addBlankIfRequired(l.RenpoPeers+l.ZeonPeers) + addBlankIfRequired(l.RenpoRoomPeers+l.ZeonRoomPeers) + addBlankIfRequired(l.NoForcePeers),
			})
		}
		payload.Embed = append(payload.Embed, &DiscordEmbed{
			Description: fmt.Sprintf("🌐 **待機中 %d 人**", lobbyPeerCount),
			Color:       24041,
			Fields:      lobbyFields,
		})

		//3rd Embed, battle count
		var battleFields []*DiscordEmbedField
		for _, b := range battle {
			battleFields = append(battleFields, &DiscordEmbedField{
				Name:  b.RegionName,
				Value: addBlankIfRequired(b.RenpoPeers + b.ZeonPeers),
			})
		}
		payload.Embed = append(payload.Embed, &DiscordEmbed{
			Description: fmt.Sprintf("💥 **戦闘中 %d 人**", battlePeerCount),
			Color:       13179394,
			Fields:      battleFields,
		})

		return payload, nil
	})

	var jsonData []byte
	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to create Discord JSON", zap.Error(err))
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
