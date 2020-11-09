package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var discordLiveStatusRetryTimer *time.Timer
var discordLiveStatusIsPublishing uint32

//PublishLiveStatusToDiscord : Update server status to the predefined Discord message thru Web Hook
func (lbs *Lbs) PublishLiveStatusToDiscord() {
	if len(conf.DiscordLiveStatusWebhookURL) == 0 {
		return
	}

	// Core function for Discord webhook
	var publish func()

	//
	// Ensure only 1 publishing job can be run, block all other requests
	//
	if atomic.CompareAndSwapUint32(&discordLiveStatusIsPublishing, 0, 1) {
		go func() {
			publish()
			atomic.StoreUint32(&discordLiveStatusIsPublishing, 0)
		}()
	} else {
		logger.Info("Request blocked!")
		if discordLiveStatusRetryTimer != nil {
			discordLiveStatusRetryTimer.Stop()
		}
		//Retry last blocked request after 0.5s
		discordLiveStatusRetryTimer = time.AfterFunc(time.Second/2, func() {
			logger.Info("Retrying!")
			publish()
		})
	}

	//
	//Type definition
	//
	type discordEmbedFooter struct {
		Text string `json:"text,omitempty"`
	}

	type discordEmbedField struct {
		Name   string `json:"name,omitempty"`
		Value  string `json:"value,omitempty"`
		Inline bool   `json:"inline,omitempty"`
	}

	type discordEmbed struct {
		Title       string               `json:"title,omitempty"`
		Description string               `json:"description,omitempty"`
		Color       int                  `json:"color,omitempty"`
		Footer      *discordEmbedFooter  `json:"footer,omitempty"`
		Timestamp   string               `json:"timestamp,omitempty"`
		Fields      []*discordEmbedField `json:"fields,omitempty"`
	}

	type statusPayload struct {
		Embed   []*discordEmbed `json:"embeds"`
		BotName string          `json:"username"`
	}

	type lobbyUsers struct {
		Count                int
		Name                 string
		RegionName           string
		Comment              string
		RenpoUsersString     string
		ZeonUsersString      string
		RenpoRoomUsersString string
		ZeonRoomUsersString  string
		NoForceUsersString   string
	}

	type battleUsers struct {
		RegionName       string
		RenpoUsersString string
		ZeonUsersString  string
	}

	//
	// Helper function
	//
	contains := func(s []string, e string) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	sortedKeys := func(m map[uint16]*lobbyUsers) []uint16 {
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
	splitEmbedFieldValues := func(value string) []string {
		var values []string

		//Split field values into 1024 chunks
		numberOfSplit := int(math.Ceil(float64(len(value)) / 1024))

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
	splitEmbedFields := func(fields []*discordEmbedField) [][]*discordEmbedField {
		var divided [][]*discordEmbedField
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

	reduceJSONStringSize := func(s string) string {
		//Reduce size by removing userid
		re := regexp.MustCompile("`.*?`\\s")
		s = re.ReplaceAllString(s, "")

		replacer := strings.NewReplacer("<:gundam:772467554160738355>", "🌎", "<:zaku:772467605008023563>", "🪐")
		s = replacer.Replace(s)

		return s
	}

	//
	// Implementation for the core function
	//
	publish = func() {
		//Stop any retry request since new request appeared
		if discordLiveStatusRetryTimer != nil {
			discordLiveStatusRetryTimer.Stop()
			discordLiveStatusRetryTimer = nil
		}

		//
		// Create battle user list, for the third embed
		//
		battle := make(map[string]*battleUsers)
		existingBattleUserIDs := make(map[string]bool)

		battleUserCount := 0
		accumulatedEmbedStringLength := 0

		for _, u := range sharedData.GetMcsUsers() {
			_, exists := battle[u.BattleCode]
			if exists == false {
				battle[u.BattleCode] = new(battleUsers)

				locName, ok := gcpLocationName[u.McsRegion]
				if !ok {
					locName = "Default Server"
				}
				if u.McsRegion == "best" {
					locName = "Best Server"
				}
				battle[u.BattleCode].RegionName = locName
				accumulatedEmbedStringLength += len(locName)
			}

			var user string
			switch u.Side {
			case TeamRenpo:
				user = fmt.Sprintf("<:gundam:772467554160738355> `%s` %s\n", u.UserID, u.Name)
				battle[u.BattleCode].RenpoUsersString += user
			case TeamZeon:
				user = fmt.Sprintf("<:zaku:772467605008023563> `%s` %s\n", u.UserID, u.Name)
				battle[u.BattleCode].ZeonUsersString += user
			}
			accumulatedEmbedStringLength += len(user)

			battleUserCount++
			existingBattleUserIDs[u.UserID] = true
		}

		//
		// Create lobby user list, for the second embed
		//
		var plazaUsers string

		lobby := make(map[uint16]*lobbyUsers)

		plazaUserCount := 0
		lobbyUserCount := 0

		lbs.Locked(func(lbs *Lbs) {
			for _, u := range lbs.userPeers {

				//Already in battle, hidden from lobby
				if existingBattleUserIDs[u.UserID] {
					continue
				}

				if u.Lobby == nil {
					plazaUsers += fmt.Sprintf("`%s` %s\n", u.UserID, u.Name)
					accumulatedEmbedStringLength += len(plazaUsers)
					plazaUserCount++
				} else {
					_, exists := lobby[u.Lobby.ID]
					if exists == false {
						lobby[u.Lobby.ID] = new(lobbyUsers)
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
						accumulatedEmbedStringLength += len(comment)
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
					var user string
					switch u.Team {
					case TeamRenpo:
						user = fmt.Sprintf("<:gundam:772467554160738355>%s `%s` %s\n", readyColor, u.UserID, u.Name)
						if u.Room == nil {
							lobby[u.Lobby.ID].RenpoUsersString += user
						} else {
							lobby[u.Lobby.ID].RenpoRoomUsersString += user
						}
					case TeamZeon:
						user = fmt.Sprintf("<:zaku:772467605008023563>%s `%s` %s\n", readyColor, u.UserID, u.Name)
						if u.Room == nil {
							lobby[u.Lobby.ID].ZeonUsersString += user
						} else {
							lobby[u.Lobby.ID].ZeonRoomUsersString += user
						}
					default:
						user = fmt.Sprintf("❔⚫ `%s` %s\n", u.UserID, u.Name)
						lobby[u.Lobby.ID].NoForceUsersString += user
					}
					accumulatedEmbedStringLength += len(user)

					lobby[u.Lobby.ID].Count++
				}
				lobbyUserCount++
			}
		})

		//
		// Start to create JSON payload
		//
		payload := new(statusPayload)
		payload.BotName = "Live Status"

		//
		//1st Embed, online count
		//
		payload.Embed = append(payload.Embed, &discordEmbed{
			Title:     fmt.Sprintf("**同時接続数 %d 人 **", lobbyUserCount+battleUserCount),
			Color:     52224,
			Footer:    &discordEmbedFooter{Text: "🕒"},
			Timestamp: fmt.Sprintf(time.Now().UTC().Format("2006-01-02T15:04:05.000Z")),
		})

		//
		//2nd Embed, lobby count
		//

		//1st Field is always Plaza
		var lobbyFields []*discordEmbedField
		if plazaUserCount > 0 {
			for i, v := range splitEmbedFieldValues(plazaUsers) {
				name := "⠀"
				if i == 0 {
					name = fmt.Sprintf("**Plaza － %d 人**", plazaUserCount)
				}

				lobbyFields = append(lobbyFields, &discordEmbedField{
					Name:  name,
					Value: v,
				})
				accumulatedEmbedStringLength += len(name) + len(v)
			}
		}

		//Following Fields would be all lobbies
		//Use sortedKeys to fix the ordering
		for _, i := range sortedKeys(lobby) {
			l := lobby[i]

			value := addBlankIfRequired(l.RenpoUsersString+l.ZeonUsersString) + addBlankIfRequired(l.RenpoRoomUsersString+l.ZeonRoomUsersString) + addBlankIfRequired(l.NoForceUsersString)

			for i, v := range splitEmbedFieldValues(value) {
				name := "⠀"
				if i == 0 {
					name = fmt.Sprintf("**%s － %d 人\n%s %s**", l.Name, l.Count, l.RegionName, l.Comment)
				}

				lobbyFields = append(lobbyFields, &discordEmbedField{
					Name:  name,
					Value: v,
				})
				accumulatedEmbedStringLength += len(name) + len(v)
			}

		}
		for i, fields := range splitEmbedFields(lobbyFields) {
			description := ""
			if i == 0 {
				description = fmt.Sprintf("🌐 **待機中 %d 人**", lobbyUserCount)
			}
			payload.Embed = append(payload.Embed, &discordEmbed{
				Description: description,
				Color:       24041,
				Fields:      fields,
			})
			accumulatedEmbedStringLength += len(description)
		}

		//
		//3rd Embed, battle count
		//
		var battleFields []*discordEmbedField
		for _, b := range battle {

			value := addBlankIfRequired(b.RenpoUsersString + b.ZeonUsersString)

			for i, v := range splitEmbedFieldValues(value) {
				name := "⠀"
				if i == 0 {
					name = b.RegionName
				}

				battleFields = append(battleFields, &discordEmbedField{
					Name:  name,
					Value: v,
				})
				accumulatedEmbedStringLength += len(name) + len(v)
			}
		}
		for i, fields := range splitEmbedFields(battleFields) {
			description := ""
			if i == 0 {
				description = fmt.Sprintf("💥 **戦闘中 %d 人**", battleUserCount)
			}
			payload.Embed = append(payload.Embed, &discordEmbed{
				Description: description,
				Color:       13179394,
				Fields:      fields,
			})
			accumulatedEmbedStringLength += len(description)
		}

		//
		// Create the json
		//
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		err := encoder.Encode(payload)

		if err != nil {
			logger.Error("Failed to create Discord JSON", zap.Error(err))
			return
		}

		//embed structure must not exceed 6000 characters
		if accumulatedEmbedStringLength > 6000 {
			originalSize := buffer.Len()

			start := time.Now()
			jsonString := reduceJSONStringSize(buffer.String())
			buffer.Reset()
			buffer.Write([]byte(jsonString))
			elapsed := time.Since(start)

			logger.Info("Reduce JSON Size!", zap.Int("original size", originalSize), zap.Any("elapsed", elapsed))
		}

		logger.Info(buffer.String(), zap.Int("length", buffer.Len()))

		//
		// Send to Discord
		//
		var send func(buf *bytes.Buffer)
		send = func(buf *bytes.Buffer) {

			req, err := http.NewRequest("PATCH", conf.DiscordLiveStatusWebhookURL, buf)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				logger.Error("Failed to publish to Discord", zap.Error(err))
				return
			}
			defer resp.Body.Close()

			logger.Info("Discord Webhook sent", zap.String("Status", resp.Status))
			if resp.StatusCode == http.StatusBadRequest {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					logger.Error("Failed to read response body", zap.Error(err))
					return
				}
				logger.Error("Discord 400 Bad Request", zap.String("Error:", string(body)))

			} else if resp.StatusCode == http.StatusTooManyRequests {

				resetepochTime := resp.Header.Get("x-ratelimit-reset")
				sec, _ := strconv.ParseInt(resetepochTime, 10, 64)
				logger.Info("Rate limit", zap.Int64("x-ratelimit-reset", sec))

				discordLiveStatusRetryTimer = time.AfterFunc(time.Unix(sec, 0).Sub(time.Now()), func() {
					logger.Info("Retrying last request", zap.Int64("epoch", time.Now().Unix()))
					send(buf)
				})
			}

		}

		send(buffer)

	}

}
