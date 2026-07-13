package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/t0mer/tollan/internal/crypto"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/notify"
)

// masked is the placeholder returned in place of stored secrets.
const masked = "***"

func (a *API) notificationRoutes(r chi.Router) {
	r.Get("/", a.handleListChannels)
	r.Post("/", a.handleChannelPut(true))
	r.Post("/test", a.handleTestChannel)
	r.Put("/{id}", a.handleChannelPut(false))
	r.Delete("/{id}", a.handleDeleteChannel)
}

func (a *API) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeJSON(w, http.StatusOK, []notify.Channel{})
		return
	}
	ents, err := a.deps.Meta.ListEntities(r.Context(), meta.KindChannel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]notify.Channel, 0, len(ents))
	for _, e := range ents {
		var ch notify.Channel
		if err := json.Unmarshal(e.Data, &ch); err == nil {
			maskChannel(&ch)
			out = append(out, ch)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleChannelPut(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Meta == nil || a.deps.Cipher == nil {
			writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
			return
		}
		var ch notify.Channel
		if err := decodeJSONValue(r, &ch); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if ch.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		id := chi.URLParam(r, "id")
		if create {
			id = uuid.NewString()
		}
		ch.ID = id

		// Preserve unchanged (masked) secrets from the stored channel.
		if !create {
			if existing, err := a.loadChannel(r.Context(), id); err == nil {
				keepMaskedSecrets(&ch, existing)
			}
		}
		encryptChannelSecrets(&ch, a.deps.Cipher)

		data, _ := json.Marshal(ch)
		ent, err := a.deps.Meta.PutEntity(r.Context(), meta.KindChannel, id, ch.Name, data)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var stored notify.Channel
		_ = json.Unmarshal(ent.Data, &stored)
		maskChannel(&stored)
		status := http.StatusOK
		if create {
			status = http.StatusCreated
		}
		writeJSON(w, status, stored)
	}
}

func (a *API) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	err := a.deps.Meta.DeleteEntity(r.Context(), meta.KindChannel, chi.URLParam(r, "id"))
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestChannel sends a real test message using the submitted config,
// resolving masked secrets from the stored channel.
func (a *API) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	if a.deps.Notifier == nil || a.deps.Cipher == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	var ch notify.Channel
	if err := decodeJSONValue(r, &ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.resolveSecrets(r.Context(), &ch)
	if err := a.deps.Notifier.Send(r.Context(), ch, "Tollan test notification ✅"); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (a *API) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeJSON(w, http.StatusOK, []meta.Event{})
		return
	}
	events, err := a.deps.Meta.ListEvents(r.Context(), parseInt(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []meta.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

// loadChannel loads and returns the stored (encrypted) channel.
func (a *API) loadChannel(ctx context.Context, id string) (notify.Channel, error) {
	ent, err := a.deps.Meta.GetEntity(ctx, meta.KindChannel, id)
	if err != nil {
		return notify.Channel{}, err
	}
	var ch notify.Channel
	err = json.Unmarshal(ent.Data, &ch)
	return ch, err
}

// resolveSecrets makes a channel's secrets usable for sending: masked values are
// replaced from storage and decrypted; explicit values are used as-is.
func (a *API) resolveSecrets(ctx context.Context, ch *notify.Channel) {
	var stored notify.Channel
	if ch.ID != "" {
		stored, _ = a.loadChannel(ctx, ch.ID)
	}
	resolve := func(field, storedVal string) string {
		if field == masked {
			dec, _ := a.deps.Cipher.Decrypt(storedVal)
			return dec
		}
		dec, _ := a.deps.Cipher.Decrypt(field)
		return dec
	}
	ch.URL = resolve(ch.URL, stored.URL)
	ch.Token = resolve(ch.Token, stored.Token)
	ch.Password = resolve(ch.Password, stored.Password)
}

func maskChannel(ch *notify.Channel) {
	if ch.URL != "" {
		ch.URL = masked
	}
	if ch.Token != "" {
		ch.Token = masked
	}
	if ch.Password != "" {
		ch.Password = masked
	}
}

// keepMaskedSecrets replaces masked incoming secrets with the stored values.
func keepMaskedSecrets(ch *notify.Channel, existing notify.Channel) {
	if ch.URL == masked {
		ch.URL = existing.URL
	}
	if ch.Token == masked {
		ch.Token = existing.Token
	}
	if ch.Password == masked {
		ch.Password = existing.Password
	}
}

func encryptChannelSecrets(ch *notify.Channel, c *crypto.Cipher) {
	ch.URL, _ = c.Encrypt(ch.URL)
	ch.Token, _ = c.Encrypt(ch.Token)
	ch.Password, _ = c.Encrypt(ch.Password)
}

// decodeJSONValue decodes a request body into a value (not strict about unknown
// fields, since channels have provider-specific shapes).
func decodeJSONValue(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}
