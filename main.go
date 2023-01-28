package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	gogpt "github.com/sashabaranov/go-gpt3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// DiscordAPIToken is the flag for the Discord API token
	DiscordAPIToken = "discord-api-token"

	// OpenAIAPIToken is the flag for the OpenAI API token
	OpenAIAPIToken = "openai-api-token"
)

var botCmd = &cobra.Command{
	Use:   "aybot",
	Short: "aybot is Aybaze's little helper bot.",
	Long:  "aybot is Aybaze's little helper bot.",
	Run:   doCmd,
}

func init() {
	cobra.OnInitialize(initConfig)

	botCmd.Flags().StringP(DiscordAPIToken, "t", "", "The token for Discord integration")
	viper.BindPFlag(DiscordAPIToken, botCmd.Flags().Lookup(DiscordAPIToken))
}

func initConfig() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

var homeChannels = make(map[string]string)
var notificationsChannels = make(map[string]string)

var presences = make(map[string]string)
var voiceStates = make(map[string]discordgo.VoiceState)
var channels = make(map[string]discordgo.Channel)

var lastVoiceMessage = make(map[string]*discordgo.Message)
var lastGameMessage = make(map[string]*discordgo.Message)

var voiceTitlesJoining = []string{"Let's talk!", "Did you know?", "Who needs TeamSpeak?", "Someone.. talk to him!"}
var voiceTitlesLeaving = []string{"Bye, bye!", "Uhm... gone already?"}

var gptClient *gogpt.Client

var myself *discordgo.User

func doCmd(cmd *cobra.Command, args []string) {
	token := viper.GetString(DiscordAPIToken)

	if token == "" {
		log.Println("No Discord API token provided.")
		return
	}

	apiKey := viper.GetString(OpenAIAPIToken)

	if apiKey == "" {
		log.Println("No OpenAI API token provided.")
		return
	}

	gptClient = gogpt.NewClient(apiKey)
	res, err := gptClient.CreateCompletion(context.Background(), gogpt.CompletionRequest{
		Model:            gogpt.GPT3TextDavinci003,
		MaxTokens:        256,
		Temperature:      0.7,
		Prompt:           "Are you ready to play?",
		TopP:             1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		BestOf:           1,
	})
	if err != nil {
		log.Printf("OpenAI integration is not working: %s", err)
	} else {
		log.Print(res.Choices[0].Text)
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Println("Error creating Discord session: ", err)
		return
	}

	// Fetch some information about myself
	myself, err = session.User("@me")
	if err != nil {
		log.Printf("Could not find out about myself: %v", err)
		return
	}

	//session.LogLevel = discordgo.LogDebug

	session.AddHandler(voiceStateUpdate)
	session.AddHandler(presenceUpdate)
	session.AddHandler(messageCreated)

	session.AddHandler(ready)

	session.AddHandler(guildCreate)

	session.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildPresences |
		discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages)

	// Open the websocket and begin listening.
	err = session.Open()
	if err != nil {
		log.Println("Error opening Discord session: ", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	log.Println("Aybot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

func main() {
	log.SetOutput(os.Stdout)

	if err := botCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// This function will be called when the bot receives the "ready" event from Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {
	// Set the playing status.
	s.UpdateGameStatus(0, "ready to serve")
}

// This function will be called when a message is created,
func messageCreated(s *discordgo.Session, m *discordgo.MessageCreate) {
	content := strings.ToLower(m.Message.Content)

	if strings.Contains(content, "egal") ||
		strings.Contains(content, "shrug") ||
		strings.Contains(content, "🤷‍♂️") ||
		strings.Contains(content, "¯\\_(ツ)_/¯") {
		if err := s.MessageReactionAdd(m.ChannelID, m.ID, "🤷‍♂️"); err != nil {
			log.Println(err)
		}
	}

	// Make sure, we don't invoke it based on our own messages
	if m.Author.ID == myself.ID {
		return
	}

	var invokeOpenAI = false
	for _, user := range m.Mentions {
		if user.ID == myself.ID {
			invokeOpenAI = true
		}
	}

	if strings.Contains(content, myself.Username) {
		invokeOpenAI = true
	}

	if invokeOpenAI {
		queryOpenAI(s, m.Message)
	}
}

func queryOpenAI(s *discordgo.Session, m *discordgo.Message) {
	content := strings.ReplaceAll(m.ContentWithMentionsReplaced(), fmt.Sprintf("@%s", myself.Username), "")
	content = strings.ReplaceAll(content, myself.Username, "")

	log.Printf("Querying OpenAI...")

	var err error
	var res gogpt.CompletionResponse
	res, err = gptClient.CreateCompletion(context.Background(), gogpt.CompletionRequest{
		Model:            gogpt.GPT3TextDavinci003,
		MaxTokens:        256,
		Temperature:      0.7,
		Prompt:           content,
		TopP:             1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		BestOf:           1,
	})
	if err != nil {
		log.Printf("Could not execute OpenAI request: %v\n", err)
		if err := s.MessageReactionAdd(m.ChannelID, m.ID, "❌"); err != nil {
			log.Println(err)
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, res.Choices[0].Text)
		log.Printf("OpenAI Usage: %d total tokens\n", res.Usage.TotalTokens)
	}
}

// This function will be called when the voice state changes,
func voiceStateUpdate(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {
	// dont update if its bot himself
	if m.UserID == s.State.User.ID {
		return
	}

	// fetch some information about the user
	member, _ := s.GuildMember(m.GuildID, m.UserID)

	log.Printf("Voice state update: %+v", m.VoiceState)

	// fetch old voice state for user
	oldState := voiceStates[m.UserID]

	// somehow, the channel has been changed
	if oldState.ChannelID != m.ChannelID {
		if m.ChannelID == "" {
			// user left
			log.Printf("%s left voice channel **%s**.", member.User.Username, channels[oldState.ChannelID].Name)

			embed, _ := renderVoiceStateText(member.User, true, &oldState)

			lastVoiceMessage[m.UserID], _ = s.ChannelMessageSendEmbed(homeChannels[m.GuildID], embed)
		} else {
			// user joined
			log.Printf("%s joined voice channel **%s**.", member.User.Username, channels[m.ChannelID].Name)

			embed, _ := renderVoiceStateText(member.User, false, m.VoiceState)

			lastVoiceMessage[m.UserID], _ = s.ChannelMessageSendEmbed(homeChannels[m.GuildID], embed)
		}
	} else {
		// some kind of other event, for example, mute/unmute
		log.Printf("Muting settings of user %s changed.", member.User.Username)

		if lastVoiceMessage[m.UserID] != nil {
			embed, _ := renderVoiceStateText(member.User, false, m.VoiceState)

			s.ChannelMessageEditEmbed(homeChannels[m.GuildID], lastVoiceMessage[m.UserID].ID, embed)
		}
	}

	// update state
	voiceStates[m.UserID] = *m.VoiceState
}

func getRandomTitle(titles []string) string {
	return titles[rand.Intn(len(titles))]
}

func renderVoiceStateText(user *discordgo.User, leaving bool, voiceState *discordgo.VoiceState) (embed *discordgo.MessageEmbed, err error) {
	if user == nil || voiceState == nil {
		return nil, errors.New("please specifiy a valid user and voice state")
	}

	var s = fmt.Sprintf("**%s** ", user.Username)

	if leaving {
		s += "left"
	} else {
		s += "joined"
	}

	s += fmt.Sprintf(" voice channel **#%s**", channels[voiceState.ChannelID].Name)

	if !leaving && voiceState != nil && voiceState.SelfDeaf {
		s += " and is *deaf*."
	} else if !leaving && voiceState != nil && voiceState.SelfMute {
		s += " and is *muted*."
	} else {
		s += "."
	}

	if leaving {
		embed = &discordgo.MessageEmbed{
			Description: s,
			Color:       0x96281b,
			Author: &discordgo.MessageEmbedAuthor{
				Name:    getRandomTitle(voiceTitlesLeaving),
				IconURL: user.AvatarURL("128"),
			},
		}
	} else {
		embed = &discordgo.MessageEmbed{
			Description: s,
			Color:       0x1e824c,
			Author: &discordgo.MessageEmbedAuthor{
				Name:    getRandomTitle(voiceTitlesJoining),
				IconURL: user.AvatarURL("128"),
			},
		}
	}

	return embed, nil
}

func presencesReplace(s *discordgo.Session, m *discordgo.PresencesReplace) {
	log.Printf("Presence replace: %+v\n", m)
}

func presenceUpdate(s *discordgo.Session, m *discordgo.PresenceUpdate) {
	log.Printf("Presence update: %+v\n", m)

	member, _ := s.GuildMember(m.GuildID, m.User.ID)

	oldPresence := presences[m.User.ID]
	newPresence := ""

	if len(m.Activities) > 0 {
		newPresence = m.Activities[0].Name
	}

	changed := oldPresence != newPresence

	// nothing really has changed, ignore
	// this is mostly if a members goes to/from afk to online, etc.
	if !changed {
		return
	}

	if newPresence == "" {
		// user stopped playing
		data := &discordgo.MessageEmbed{
			Description: fmt.Sprintf("... stopped playing **%s**.", oldPresence),
			Color:       0x96281b,
			//Timestamp: time.Now().Format(time.RFC3339),
			Author: &discordgo.MessageEmbedAuthor{Name: member.User.Username, IconURL: member.User.AvatarURL("128")},
		}

		_, _ = s.ChannelMessageSendEmbed(notificationsChannels[m.GuildID], data)
	} else if oldPresence == "" {
		// user started playing (he did not play before)
		data := &discordgo.MessageEmbed{
			Description: fmt.Sprintf("... started playing **%s**.", newPresence),
			Color:       0x1e824c,
			//Timestamp: time.Now().Format(time.RFC3339),
			Author: &discordgo.MessageEmbedAuthor{Name: member.User.Username, IconURL: member.User.AvatarURL("128")},
		}

		lastGameMessage[m.User.ID], _ = s.ChannelMessageSendEmbed(notificationsChannels[m.GuildID], data)
	} else {
		// user just switched gaming
		data := &discordgo.MessageEmbed{
			Description: fmt.Sprintf("... is now playing **%s**.", newPresence),
			Color:       0x1e824c,
			//Timestamp: time.Now().Format(time.RFC3339),
			Author: &discordgo.MessageEmbedAuthor{Name: member.User.Username, IconURL: member.User.AvatarURL("128")},
		}

		s.ChannelMessageEditEmbed(notificationsChannels[m.GuildID], lastGameMessage[m.User.ID].ID, data)
	}

	// update presence
	presences[m.User.ID] = newPresence
}

// This function will be called (due to AddHandler above) every time a new
// guild is joined.
func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	// update voice states
	for _, voiceState := range event.Guild.VoiceStates {
		if voiceState != nil {
			log.Printf("Updating voice state for %s.", voiceState.UserID)

			voiceState.GuildID = event.Guild.ID

			voiceStates[voiceState.UserID] = *voiceState
		}
	}

	// update presences
	for _, presence := range event.Guild.Presences {
		if len(presence.Activities) > 0 {
			presences[presence.User.ID] = presence.Activities[0].Name
		}
	}

	// update channels
	for _, channel := range event.Guild.Channels {
		if channel != nil {
			channels[channel.ID] = *channel
		}
	}

	for _, channel := range event.Guild.Channels {
		if channel.Name == "naughtyfications" {
			// _, _ = s.ChannelMessageSend(channel.ID, "Aybot is ready to serve, biatch!")
			notificationsChannels[event.Guild.ID] = channel.ID
		}

		if channel.Name == "hampel" {
			// _, _ = s.ChannelMessageSend(channel.ID, "Aybot is ready to serve, biatch!")
			homeChannels[event.Guild.ID] = channel.ID
		}
	}
}
