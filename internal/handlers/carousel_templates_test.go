package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func carouselCard(body, mediaID string) map[string]any {
	return map[string]any{
		"header_format": "IMAGE",
		"header_handle": "4::" + mediaID,
		"media_id":      mediaID,
		"filename":      mediaID + ".jpg",
		"body":          body,
		"buttons": []any{
			map[string]any{"type": "URL", "text": "Book", "url": "https://gharit.in/book"},
		},
	}
}

func carouselRequest(accountName string) map[string]any {
	return map[string]any{
		"whatsapp_account": accountName,
		"name":             "Our Services",
		"language":         "en",
		"category":         "MARKETING",
		"header_type":      "CAROUSEL",
		"body_content":     "Hi {{1}}, here's what we care for",
		"sample_values":    []any{map[string]any{"component": "body", "index": 1, "value": "Priya"}},
		"carousel_cards": []any{
			carouselCard("Shirts, pressed", "m1"),
			carouselCard("Sarees, dry cleaned", "m2"),
		},
	}
}

func TestApp_CreateTemplate_Carousel(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)

	req := testutil.NewJSONRequest(t, carouselRequest(account.Name))
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.CreateTemplate(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))

	var resp struct {
		Data handlers.TemplateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, "CAROUSEL", resp.Data.HeaderType)
	cards := whatsapp.ParseCarouselCards(resp.Data.CarouselCards)
	require.Len(t, cards, 2)
	assert.Equal(t, "m2", cards[1].MediaID)

	// A carousel with one card is refused.
	bad := carouselRequest(account.Name)
	bad["name"] = "one card"
	bad["carousel_cards"] = bad["carousel_cards"].([]any)[:1]
	req = testutil.NewJSONRequest(t, bad)
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.CreateTemplate(req))
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))
	assert.Contains(t, string(testutil.GetResponseBody(req)), "2 to 10 cards")
}

func TestApp_UpdateTemplate_LeavingCarouselDropsCards(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)

	tmpl := createTestTemplateInDB(t, app, org.ID, account.Name, "was_carousel", "DRAFT")
	tmpl.HeaderType = "CAROUSEL"
	tmpl.CarouselCards = models.JSONBArray{carouselCard("a", "m1"), carouselCard("b", "m2")}
	require.NoError(t, app.DB.Save(tmpl).Error)

	req := testutil.NewJSONRequest(t, map[string]any{"header_type": "NONE", "body_content": "Plain now"})
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", tmpl.ID.String())
	require.NoError(t, app.UpdateTemplate(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var saved models.Template
	require.NoError(t, app.DB.First(&saved, "id = ?", tmpl.ID).Error)
	assert.Equal(t, "NONE", saved.HeaderType)
	assert.Empty(t, saved.CarouselCards)
}

func TestApp_SubmitTemplate_Carousel(t *testing.T) {
	t.Parallel()

	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "meta-carousel-1"})
	}))
	defer server.Close()
	app := newTemplateTestApp(t, server)

	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)

	create := testutil.NewJSONRequest(t, carouselRequest(account.Name))
	testutil.SetAuthContext(create, org.ID, user.ID)
	require.NoError(t, app.CreateTemplate(create))
	var created struct {
		Data handlers.TemplateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(create), &created))

	req := testutil.NewJSONRequest(t, nil)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", created.Data.ID.String())
	require.NoError(t, app.SubmitTemplate(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))

	components := sent["components"].([]any)
	require.Len(t, components, 2)
	assert.Equal(t, "BODY", components[0].(map[string]any)["type"])
	carousel := components[1].(map[string]any)
	assert.Equal(t, "CAROUSEL", carousel["type"])
	assert.Len(t, carousel["cards"], 2)
}

func TestApp_SubmitTemplate_CarouselNeedsSamples(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not reach Meta without card samples")
	}))
	defer server.Close()
	app := newTemplateTestApp(t, server)

	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)

	tmpl := createTestTemplateInDB(t, app, org.ID, account.Name, "no_samples", "DRAFT")
	noSample := carouselCard("b", "m2")
	delete(noSample, "header_handle")
	tmpl.HeaderType = "CAROUSEL"
	tmpl.CarouselCards = models.JSONBArray{carouselCard("a", "m1"), noSample}
	require.NoError(t, app.DB.Save(tmpl).Error)

	req := testutil.NewJSONRequest(t, nil)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", tmpl.ID.String())
	require.NoError(t, app.SubmitTemplate(req))
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))
	assert.Contains(t, string(testutil.GetResponseBody(req)), "card 2: upload a sample image")
}

func TestApp_SyncTemplates_CarouselKeepsMediaIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
			"id": "meta-carousel-1", "name": "our_services", "language": "en",
			"category": "MARKETING", "status": "APPROVED",
			"components": []map[string]any{
				{"type": "BODY", "text": "Hi {{1}}, here's what we care for"},
				{"type": "CAROUSEL", "cards": []map[string]any{
					{"components": []map[string]any{
						{"type": "HEADER", "format": "IMAGE", "example": map[string]any{"header_handle": []string{"https://scontent/1.jpg"}}},
						{"type": "BODY", "text": "Shirts, pressed"},
						{"type": "BUTTONS", "buttons": []map[string]any{{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}}},
					}},
					{"components": []map[string]any{
						{"type": "HEADER", "format": "IMAGE", "example": map[string]any{"header_handle": []string{"https://scontent/2.jpg"}}},
						{"type": "BODY", "text": "Sarees, dry cleaned"},
						{"type": "BUTTONS", "buttons": []map[string]any{{"type": "URL", "text": "Book", "url": "https://gharit.in/book"}}},
					}},
				}},
			},
		}}})
	}))
	defer server.Close()
	app := newTemplateTestApp(t, server)

	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)

	local := createTestTemplateInDB(t, app, org.ID, account.Name, "our_services", "PENDING")
	local.HeaderType = "CAROUSEL"
	local.CarouselCards = models.JSONBArray{carouselCard("Shirts, pressed", "m1"), carouselCard("Sarees, dry cleaned", "m2")}
	require.NoError(t, app.DB.Save(local).Error)

	req := testutil.NewJSONRequest(t, map[string]any{"whatsapp_account": account.Name})
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.SyncTemplates(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var saved models.Template
	require.NoError(t, app.DB.First(&saved, "id = ?", local.ID).Error)
	assert.Equal(t, "APPROVED", saved.Status)
	assert.Equal(t, "CAROUSEL", saved.HeaderType)
	cards := whatsapp.ParseCarouselCards(saved.CarouselCards)
	require.Len(t, cards, 2)
	assert.Equal(t, "https://scontent/2.jpg", cards[1].HeaderHandle)
	assert.Equal(t, "m2", cards[1].MediaID)
}

func TestApp_SendTemplateMessage_Carousel(t *testing.T) {
	t.Parallel()
	mockServer := newMockWhatsAppServer()
	defer mockServer.close()

	app := newMsgTestApp(t, mockServer)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := createTestAccount(t, app, org.ID)
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))

	tpl := createTestTemplate(t, app, org.ID, account.Name)
	tpl.BodyContent = "Hi {{name}}, here's what we care for"
	tpl.HeaderType = "CAROUSEL"
	tpl.CarouselCards = models.JSONBArray{carouselCard("Shirts, pressed", "m1"), carouselCard("Sarees, dry cleaned", "m2")}
	require.NoError(t, app.DB.Save(tpl).Error)

	req := testutil.NewJSONRequest(t, map[string]any{
		"contact_id":      contact.ID.String(),
		"template_name":   tpl.Name,
		"template_params": map[string]string{"name": "Priya"},
	})
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.SendTemplateMessage(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))
	app.WaitForBackgroundTasks()

	require.Len(t, mockServer.sentMessages, 1)
	components := mockServer.sentMessages[0]["template"].(map[string]any)["components"].([]any)
	var carousel map[string]any
	for _, c := range components {
		if m := c.(map[string]any); m["type"] == "carousel" {
			carousel = m
		}
	}
	require.NotNil(t, carousel, "carousel component sent")
	cards := carousel["cards"].([]any)
	require.Len(t, cards, 2)
	header := cards[1].(map[string]any)["components"].([]any)[0].(map[string]any)
	assert.Equal(t, []any{map[string]any{"type": "image", "image": map[string]any{"id": "m2"}}}, header["parameters"])
}
