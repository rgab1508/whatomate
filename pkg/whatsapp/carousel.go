package whatsapp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Media card carousel templates: a body bubble followed by 2–10 swipeable
// cards. Each card has an IMAGE or VIDEO header, its own body text and one or
// two buttons. Meta requires every card to use the same header format and the
// same button types in the same order.
//
// Cards are stored on the template as a JSON array (Template.CarouselCards)
// with this shape:
//
//	{
//	  "header_format": "IMAGE",          // IMAGE or VIDEO, same on every card
//	  "header_handle": "4::aW1h...",     // resumable-upload handle, for approval
//	  "media_id":      "1234567890",     // media ID, for sending (Meta keeps it 30 days)
//	  "media_link":    "https://...",    // public URL, for sending; preferred, never expires
//	  "filename":      "shirts.jpg",
//	  "body":          "Shirts, pressed and folded.",
//	  "buttons":       [{"type": "URL", "text": "Book", "url": "https://..."}]
//	}
//
// Card bodies and button URLs are static: no {{variables}} on cards yet. The
// main body keeps full variable support.

// HeaderTypeCarousel marks a template whose content is a card carousel. The
// template then has no top-level header, footer or buttons.
const HeaderTypeCarousel = "CAROUSEL"

const (
	carouselMinCards    = 2
	carouselMaxCards    = 10
	carouselMaxBody     = 1024
	carouselMaxCardBody = 160
	carouselMaxButtons  = 2
	carouselMaxBtnText  = 25
)

// CarouselCard is one card, read from its stored JSON.
type CarouselCard struct {
	HeaderFormat string
	HeaderHandle string
	MediaID      string
	MediaLink    string
	Filename     string
	Body         string
	Buttons      []CarouselButton
}

// CarouselButton is one button on a card.
type CarouselButton struct {
	Type        string
	Text        string
	URL         string
	PhoneNumber string
}

// ParseCarouselCards reads cards from their stored JSON form.
func ParseCarouselCards(raw []any) []CarouselCard {
	cards := make([]CarouselCard, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		card := CarouselCard{
			HeaderFormat: strings.ToUpper(str(m["header_format"])),
			HeaderHandle: str(m["header_handle"]),
			MediaID:      str(m["media_id"]),
			MediaLink:    str(m["media_link"]),
			Filename:     str(m["filename"]),
			Body:         str(m["body"]),
		}
		if btns, ok := m["buttons"].([]any); ok {
			for _, b := range btns {
				bm, ok := b.(map[string]any)
				if !ok {
					continue
				}
				card.Buttons = append(card.Buttons, CarouselButton{
					Type:        strings.ToUpper(str(bm["type"])),
					Text:        str(bm["text"]),
					URL:         str(bm["url"]),
					PhoneNumber: str(bm["phone_number"]),
				})
			}
		}
		cards = append(cards, card)
	}
	return cards
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// ValidateCarousel checks a carousel against Meta's rules. With requireMedia,
// every card must also have its sample uploaded (needed to submit to Meta).
func ValidateCarousel(body string, raw []any, requireMedia bool) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("carousel body is required")
	}
	if utf8.RuneCountInString(body) > carouselMaxBody {
		return fmt.Errorf("carousel body can be at most %d characters", carouselMaxBody)
	}
	cards := ParseCarouselCards(raw)
	if len(cards) < carouselMinCards || len(cards) > carouselMaxCards {
		return fmt.Errorf("a carousel needs %d to %d cards, found %d", carouselMinCards, carouselMaxCards, len(cards))
	}
	first := cards[0]
	for i, c := range cards {
		n := i + 1
		if c.HeaderFormat != "IMAGE" && c.HeaderFormat != "VIDEO" {
			return fmt.Errorf("card %d: header must be IMAGE or VIDEO", n)
		}
		if c.HeaderFormat != first.HeaderFormat {
			return fmt.Errorf("card %d: every card must use the same header (%s)", n, first.HeaderFormat)
		}
		if requireMedia && c.HeaderHandle == "" {
			return fmt.Errorf("card %d: upload a sample %s first", n, strings.ToLower(c.HeaderFormat))
		}
		if c.Body == "" {
			return fmt.Errorf("card %d: text is required", n)
		}
		if utf8.RuneCountInString(c.Body) > carouselMaxCardBody {
			return fmt.Errorf("card %d: text can be at most %d characters", n, carouselMaxCardBody)
		}
		if strings.Contains(c.Body, "{{") {
			return fmt.Errorf("card %d: variables aren't supported in card text yet", n)
		}
		if len(c.Buttons) < 1 || len(c.Buttons) > carouselMaxButtons {
			return fmt.Errorf("card %d: needs 1 or %d buttons", n, carouselMaxButtons)
		}
		if len(c.Buttons) != len(first.Buttons) {
			return fmt.Errorf("card %d: every card must have the same number of buttons", n)
		}
		for j, b := range c.Buttons {
			bn := j + 1
			if b.Type != first.Buttons[j].Type {
				return fmt.Errorf("card %d, button %d: buttons must be the same type, in the same order, on every card", n, bn)
			}
			if b.Text == "" {
				return fmt.Errorf("card %d, button %d: text is required", n, bn)
			}
			if utf8.RuneCountInString(b.Text) > carouselMaxBtnText {
				return fmt.Errorf("card %d, button %d: text can be at most %d characters", n, bn, carouselMaxBtnText)
			}
			switch b.Type {
			case "QUICK_REPLY":
			case "URL":
				if b.URL == "" {
					return fmt.Errorf("card %d, button %d: URL is required", n, bn)
				}
				if strings.Contains(b.URL, "{{") {
					return fmt.Errorf("card %d, button %d: variables aren't supported in card URLs yet", n, bn)
				}
			case "PHONE_NUMBER":
				if b.PhoneNumber == "" {
					return fmt.Errorf("card %d, button %d: phone number is required", n, bn)
				}
			default:
				return fmt.Errorf("card %d, button %d: type must be QUICK_REPLY, URL or PHONE_NUMBER", n, bn)
			}
		}
	}
	return nil
}

// buildCarouselComponents builds the Meta components for submitting a
// carousel template: BODY, then CAROUSEL.
func (c *Client) buildCarouselComponents(template *TemplateSubmission) ([]map[string]any, error) {
	if err := ValidateCarousel(template.BodyContent, template.CarouselCards, true); err != nil {
		return nil, err
	}
	// The body bubble follows the same rules as any other template body.
	standard, err := c.buildStandardComponents(&TemplateSubmission{
		ParameterFormat: template.ParameterFormat,
		BodyContent:     template.BodyContent,
		SampleValues:    template.SampleValues,
	})
	if err != nil {
		return nil, err
	}

	cards := ParseCarouselCards(template.CarouselCards)
	metaCards := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		buttons := make([]map[string]any, 0, len(card.Buttons))
		for _, b := range card.Buttons {
			btn := map[string]any{"type": b.Type, "text": b.Text}
			switch b.Type {
			case "URL":
				btn["url"] = b.URL
			case "PHONE_NUMBER":
				btn["phone_number"] = b.PhoneNumber
			}
			buttons = append(buttons, btn)
		}
		metaCards = append(metaCards, map[string]any{
			"components": []map[string]any{
				{
					"type":    "HEADER",
					"format":  card.HeaderFormat,
					"example": map[string]any{"header_handle": []string{card.HeaderHandle}},
				},
				{"type": "BODY", "text": card.Body},
				{"type": "BUTTONS", "buttons": buttons},
			},
		})
	}

	return append(standard, map[string]any{
		"type":  "CAROUSEL",
		"cards": metaCards,
	}), nil
}

// CarouselSendComponent builds the "carousel" component for sending a carousel
// template: each card's header media (its public link when set, else its
// uploaded media ID), plus a payload for its quick replies.
func CarouselSendComponent(raw []any) (map[string]any, error) {
	cards := ParseCarouselCards(raw)
	if len(cards) == 0 {
		return nil, fmt.Errorf("carousel template has no cards")
	}
	sendCards := make([]map[string]any, 0, len(cards))
	for i, card := range cards {
		var source map[string]any
		switch {
		case card.MediaLink != "":
			source = map[string]any{"link": card.MediaLink}
		case card.MediaID != "":
			source = map[string]any{"id": card.MediaID}
		default:
			return nil, fmt.Errorf("card %d has no media to send; upload its %s again or give it a link", i+1, strings.ToLower(card.HeaderFormat))
		}
		media := strings.ToLower(card.HeaderFormat)
		components := []map[string]any{{
			"type": "header",
			"parameters": []map[string]any{{
				"type": media,
				media:  source,
			}},
		}}
		for j, b := range card.Buttons {
			if b.Type != "QUICK_REPLY" {
				continue
			}
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "quick_reply",
				"index":    fmt.Sprintf("%d", j),
				"parameters": []map[string]any{
					{"type": "payload", "payload": b.Text},
				},
			})
		}
		sendCards = append(sendCards, map[string]any{
			"card_index": i,
			"components": components,
		})
	}
	return map[string]any{"type": "carousel", "cards": sendCards}, nil
}

// CarouselCardsFromMeta converts a synced CAROUSEL component into stored
// cards. Meta doesn't return media IDs, so each card keeps the media ID and
// link and filename it had locally (matched by position).
func CarouselCardsFromMeta(comp TemplateComponent, existing []any) []any {
	local := ParseCarouselCards(existing)
	cards := make([]any, 0, len(comp.Cards))
	for i, mc := range comp.Cards {
		card := map[string]any{"buttons": []any{}}
		for _, cc := range mc.Components {
			switch strings.ToUpper(cc.Type) {
			case "HEADER":
				card["header_format"] = strings.ToUpper(cc.Format)
				if cc.Example != nil && len(cc.Example.HeaderHandle) > 0 {
					card["header_handle"] = cc.Example.HeaderHandle[0]
				}
			case "BODY":
				card["body"] = cc.Text
			case "BUTTONS":
				buttons := make([]any, 0, len(cc.Buttons))
				for _, b := range cc.Buttons {
					btn := map[string]any{"type": b.Type, "text": b.Text}
					if b.URL != "" {
						btn["url"] = b.URL
					}
					if b.PhoneNumber != "" {
						btn["phone_number"] = b.PhoneNumber
					}
					buttons = append(buttons, btn)
				}
				card["buttons"] = buttons
			}
		}
		if i < len(local) {
			card["media_id"] = local[i].MediaID
			card["media_link"] = local[i].MediaLink
			card["filename"] = local[i].Filename
		}
		cards = append(cards, card)
	}
	return cards
}
