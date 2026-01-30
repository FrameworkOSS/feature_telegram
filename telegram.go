package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/FrameworkOSS/event"
	commands "github.com/FrameworkOSS/feature_commands"
	debugger "github.com/FrameworkOSS/feature_debugger"

	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Telegram struct {
	token string
	bot   *tg.Bot
	ctx   context.Context

	lockResp sync.Mutex
	resps    []*event.Event
	c        *commands.Commands
}

func (t *Telegram) handlerPortal(e *event.Event) error {
	fmt.Printf("%s\n", debugger.DebugEvent(e))
	if channel := e.GetChannel(); channel != "" {
		ptr := strings.Split(channel, ":")
		if len(ptr) == 2 && ptr[0] == t.ID() {
			chatID := ptr[1]
			t.bot.SendMessage(t.ctx, &tg.SendMessageParams{
				ChatID:    chatID,
				Text:      string(e.GetData()),
				ParseMode: models.ParseModeHTML,
			})
		}
	}
	return nil
}

func (t *Telegram) handlerTelegram(ctx context.Context, b *tg.Bot, update *models.Update) {
	if update != nil {
		if msg := update.Message; msg != nil {
			if msg.From != nil && msg.Text != "" {
				if msg.From.IsBot {
					return
				}
				text := msg.Text
				if text[0] == '/' {
					text = text[1:]
				}
				channel := fmt.Sprintf("%s:%d", t.ID(), msg.Chat.ID)
				e, err := t.c.NewCommandLineEvent(t.ID(), strings.Split(text, " ")...)
				if err != nil {
					go t.storeResp(event.NewEventError(t.ID(), err).SetChannel(channel))
					return
				}
				e.SetChannel(channel)
				go t.storeResp(e)
				fmt.Printf("@%s: %s\n", msg.From.Username, msg.Text)
				return
			}
		}

		if inline := update.InlineQuery; inline != nil {
			if inline.From != nil && inline.Query != "" {
				q := inline.Query
				fmt.Printf("@%s: %s\n", inline.From.Username, q)
				if q[0] == '/' {
					q = q[1:]
				}
				matches := make([]string, 0)
				for _, cmd := range t.c.Commands() {
					if q == cmd[:len(q)] {
						matches = append(matches, cmd)
					}
				}
				results := make([]models.InlineQueryResult, len(matches))
				for i := 0; i < len(matches); i++ {
					cmd := t.c.Command(matches[i])
					results[i] = &models.InlineQueryResultArticle{
						ID:          strconv.Itoa(i + 1),
						Title:       cmd.GetID(),
						Description: cmd.GetAbout(),
						InputMessageContent: &models.InputTextMessageContent{
							MessageText: cmd.GetUsage(),
						},
					}
				}
				b.AnswerInlineQuery(ctx, &tg.AnswerInlineQueryParams{
					InlineQueryID: inline.ID,
					Results:       results,
				})
				return
			}
		}
	}
}

func NewTelegram(token string, c *commands.Commands) *Telegram {
	t := new(Telegram)
	t.token = token
	t.resps = make([]*event.Event, 0)
	t.c = c
	return t
}

func (t *Telegram) API() int {
	return 0
}

func (t *Telegram) ID() string {
	return "telegram"
}

func (t *Telegram) Name() string {
	return "Telegram"
}

func (t *Telegram) Authors() []string {
	return []string{"JoshuaDoes"}
}

func (t *Telegram) Description() string {
	return "Provides a bridge to a Telegram account to serve a command transport."
}

func (t *Telegram) Version() string {
	return "v0.0.1"
}

func (t *Telegram) Open() (err error) {
	opts := []tg.Option{
		tg.WithDefaultHandler(t.handlerTelegram),
	}

	bot, err := tg.New(t.token, opts...)
	if err != nil {
		return err
	}

	ctx := context.Background()
	go bot.Start(ctx)

	t.bot = bot
	t.ctx = ctx

	t.storeResp(event.NewEventReady(t.ID(), true))

	return
}

func (t *Telegram) Close() (errs []error, retry bool) {
	if t.bot == nil {
		return []error{fmt.Errorf("telegram: no open bot to close")}, false
	}
	fail, err := t.bot.Close(t.ctx)
	if err != nil {
		return []error{fmt.Errorf("telegram: failed to close: %v", err)}, true
	}
	if fail {
		return []error{fmt.Errorf("telegram: got false when suggesting to close")}, true
	}
	return
}

func (t *Telegram) Input(e *event.Event) error {
	go t.handlerPortal(e)
	return nil
}

func (t *Telegram) Output() (e *event.Event, err error) {
	e = t.readResp()
	return
}

func (t *Telegram) storeResp(e *event.Event) {
	e.SetProducer(t.ID())
	t.lockResp.Lock()
	defer t.lockResp.Unlock()
	t.resps = append(t.resps, e)
}

func (t *Telegram) readResp() (e *event.Event) {
	t.lockResp.Lock()
	defer t.lockResp.Unlock()
	if len(t.resps) > 0 {
		e = t.resps[0]
		t.resps = t.resps[1:]
	}
	return
}
