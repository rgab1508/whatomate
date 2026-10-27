package whatsapp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func card(body string, buttons ...map[string]any) map[string]any {
	btns := make([]any, len(buttons))
	for i, b := range buttons {
		btns[i] = b
	}
	return map[string]any{
		"header_format": "IMAGE",
		"header_handle": "4::handle-" + body,
		"media_id":      "media-" + body,
		"body":          body,
		"buttons":       btns,
	}
}

func book() map[string]any {
	return map[string]any{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}
}

func reply(text string) map[string]any {
	return map[string]any{"type": "QUICK_REPLY", "text": text}
}

func twoCards() []any {
	return []any{
		card("Shirts, pressed", book(), reply("Price?")),
		card("Sarees, dry cleaned", book(), reply("Price?")),
	}
}

func TestValidateCarousel(t *testing.T) {
	t.Parallel()

	require.NoError(t, whatsapp.ValidateCarousel("Hi {{1}}, our services", twoCards(), true))

	cases := map[string]struct {
		body  string
		cards []any
		want  string
	}{
		"no body":        {"", twoCards(), "body is required"},
		"one card":       {"Hi", twoCards()[:1], "2 to 10 cards"},
		"mixed headers":  {"Hi", []any{card("a", book()), func() map[string]any { c := card("b", book()); c["header_format"] = "VIDEO"; return c }()}, "same header"},
		"no buttons":     {"Hi", []any{card("a"), card("b")}, "needs 1 or 2 buttons"},
		"button count":   {"Hi", []any{card("a", book()), card("b", book(), reply("x"))}, "same number of buttons"},
		"button order":   {"Hi", []any{card("a", book(), reply("x")), card("b", reply("x"), book())}, "same type"},
		"card variable":  {"Hi", []any{card("a {{1}}", book()), card("b", book())}, "variables aren't supported"},
		"long card text": {"Hi", []any{card(strings.Repeat("x", 161), book()), card("b", book())}, "at most 160"},
	}
	for name, tc := range cases {
		err := whatsapp.ValidateCarousel(tc.body, tc.cards, true)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.want, name)
	}

	// Drafts can be saved before the samples are uploaded, but not submitted.
	noMedia := twoCards()
	delete(noMedia[0].(map[string]any), "header_handle")
	require.NoError(t, whatsapp.ValidateCarousel("Hi", noMedia, false))
	assert.ErrorContains(t, whatsapp.ValidateCarousel("Hi", noMedia, true), "upload a sample image")
}

func TestClient_SubmitTemplate_Carousel(t *testing.T) {
	t.Parallel()

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "tmpl-carousel"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	id, err := client.SubmitTemplate(context.Background(), testAccount(server.URL), &whatsapp.TemplateSubmission{
		Name:          "services_carousel",
		Language:      "en",
		Category:      "MARKETING",
		HeaderType:    "CAROUSEL",
		BodyContent:   "Hi {{1}}, here's what we care for",
		SampleValues:  []any{map[string]any{"component": "body", "index": float64(1), "value": "Priya"}},
		CarouselCards: twoCards(),
	})
	require.NoError(t, err)
	assert.Equal(t, "tmpl-carousel", id)

	components := body["components"].([]any)
	require.Len(t, components, 2, "BODY then CAROUSEL, no top-level header/footer/buttons")
	bodyComp := components[0].(map[string]any)
	assert.Equal(t, "BODY", bodyComp["type"])
	assert.Equal(t, []any{[]any{"Priya"}}, bodyComp["example"].(map[string]any)["body_text"])

	carousel := components[1].(map[string]any)
	assert.Equal(t, "CAROUSEL", carousel["type"])
	cards := carousel["cards"].([]any)
	require.Len(t, cards, 2)
	first := cards[0].(map[string]any)["components"].([]any)
	header := first[0].(map[string]any)
	assert.Equal(t, "HEADER", header["type"])
	assert.Equal(t, "IMAGE", header["format"])
	assert.Equal(t, []any{"4::handle-Shirts, pressed"}, header["example"].(map[string]any)["header_handle"])
	assert.Equal(t, map[string]any{"type": "BODY", "text": "Shirts, pressed"}, first[1])
	buttons := first[2].(map[string]any)["buttons"].([]any)
	assert.Equal(t, map[string]any{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}, buttons[0])
	assert.Equal(t, map[string]any{"type": "QUICK_REPLY", "text": "Price?"}, buttons[1])
}

func TestClient_SubmitTemplate_CarouselRejectedLocally(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an invalid carousel must not reach Meta")
	}))
	defer server.Close()

	_, err := newTestClient(t, server).SubmitTemplate(context.Background(), testAccount(server.URL), &whatsapp.TemplateSubmission{
		Name: "bad", Language: "en", Category: "MARKETING", HeaderType: "CAROUSEL",
		BodyContent: "Hi", CarouselCards: twoCards()[:1],
	})
	assert.ErrorContains(t, err, "2 to 10 cards")
}

func TestCarouselSendComponent(t *testing.T) {
	t.Parallel()

	comp, err := whatsapp.CarouselSendComponent(twoCards())
	require.NoError(t, err)
	assert.Equal(t, "carousel", comp["type"])
	cards := comp["cards"].([]map[string]any)
	require.Len(t, cards, 2)
	assert.Equal(t, 1, cards[1]["card_index"])

	components := cards[0]["components"].([]map[string]any)
	require.Len(t, components, 2, "header + the quick reply; the static URL button needs nothing")
	assert.Equal(t, []map[string]any{{"type": "image", "image": map[string]any{"id": "media-Shirts, pressed"}}}, components[0]["parameters"])
	assert.Equal(t, "quick_reply", components[1]["sub_type"])
	assert.Equal(t, "1", components[1]["index"])

	noMedia := twoCards()
	delete(noMedia[1].(map[string]any), "media_id")
	_, err = whatsapp.CarouselSendComponent(noMedia)
	assert.ErrorContains(t, err, "card 2 has no media")

	// A public link wins over the media ID: uploaded media expires after 30 days.
	linked := twoCards()
	linked[0].(map[string]any)["media_link"] = "https://gharitapp.com/assets/Press_Steam.png"
	comp, err = whatsapp.CarouselSendComponent(linked)
	require.NoError(t, err)
	header := comp["cards"].([]map[string]any)[0]["components"].([]map[string]any)[0]
	assert.Equal(t, []map[string]any{{"type": "image", "image": map[string]any{"link": "https://gharitapp.com/assets/Press_Steam.png"}}}, header["parameters"])
}

func TestCarouselCardsFromMeta_KeepsLocalMedia(t *testing.T) {
	t.Parallel()

	var comp whatsapp.TemplateComponent
	require.NoError(t, json.Unmarshal([]byte(`{
	  "type": "CAROUSEL",
	  "cards": [
	    {"components": [
	      {"type": "HEADER", "format": "IMAGE", "example": {"header_handle": ["https://scontent/1.jpg"]}},
	      {"type": "BODY", "text": "Shirts, pressed"},
	      {"type": "BUTTONS", "buttons": [{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}]}
	    ]},
	    {"components": [
	      {"type": "HEADER", "format": "IMAGE", "example": {"header_handle": ["https://scontent/2.jpg"]}},
	      {"type": "BODY", "text": "Sarees, dry cleaned"},
	      {"type": "BUTTONS", "buttons": [{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}]}
	    ]}
	  ]
	}`), &comp))

	cards := whatsapp.ParseCarouselCards(whatsapp.CarouselCardsFromMeta(comp, twoCards()))
	require.Len(t, cards, 2)
	assert.Equal(t, "Sarees, dry cleaned", cards[1].Body)
	assert.Equal(t, "https://scontent/2.jpg", cards[1].HeaderHandle)
	assert.Equal(t, "media-Sarees, dry cleaned", cards[1].MediaID, "Meta doesn't return media IDs, so the local one stays")
	assert.Equal(t, []whatsapp.CarouselButton{{Type: "URL", Text: "Book", URL: "https://gharit.in/book"}}, cards[0].Buttons)
}
