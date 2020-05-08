package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

type SecurityFrontend struct {
	OpenId                OpenIdSettings
	Protected             http.Handler
	TokenGenerator        *TokenGenerator
	GlobalTokenValidity   int
	PerAlbumTokenValidity int

	store        *sessions.CookieStore
	oAuth2Config *oauth2.Config
	oidcVerifier *oidc.IDTokenVerifier
}

type SessionSettings struct {
	AuthenticationKey []byte
	EncryptionKey     []byte
	CookieMaxAge      int
	SecureCookie      bool
}

type OpenIdSettings struct {
	DiscoveryUrl string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	GSuiteDomain string
	Scopes       []string
}

func init() {
	gob.Register(&WebUser{})
}

func GetOAuthCallbackURL(publicUrl string) string {
	u, err := url.Parse(publicUrl)
	if err != nil {
		// If the URL cannot be parsed, use it as-is
		return publicUrl
	}

	u.Path = "/oauth/callback"
	u.Fragment = ""
	u.RawQuery = ""

	return u.String()
}

func NewSecurityFrontend(openidSettings OpenIdSettings, sessionSettings SessionSettings, tokenGenerator *TokenGenerator) (*SecurityFrontend, error) {
	var securityFrontend SecurityFrontend
	provider, err := oidc.NewProvider(context.TODO(), openidSettings.DiscoveryUrl)
	if err != nil {
		return nil, err
	}

	securityFrontend.oAuth2Config = &oauth2.Config{
		ClientID:     openidSettings.ClientID,
		ClientSecret: openidSettings.ClientSecret,
		RedirectURL:  openidSettings.RedirectURL,

		// Discovery returns the OAuth2 endpoints.
		Endpoint: provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		Scopes: append(openidSettings.Scopes, oidc.ScopeOpenID),
	}
	securityFrontend.oidcVerifier = provider.Verifier(&oidc.Config{ClientID: openidSettings.ClientID})
	securityFrontend.store = sessions.NewCookieStore(sessionSettings.AuthenticationKey, sessionSettings.EncryptionKey)
	securityFrontend.store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   sessionSettings.CookieMaxAge,
		HttpOnly: true,
		Secure:   sessionSettings.SecureCookie,
	}

	securityFrontend.OpenId = openidSettings
	securityFrontend.TokenGenerator = tokenGenerator

	return &securityFrontend, nil
}

func (securityFrontend *SecurityFrontend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originalPath := r.URL.Path
	if r.URL.Path == "/oauth/callback" {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		securityFrontend.handleOidcCallback(w, r)
		return
	}

	head, tail := ShiftPath(r.URL.Path)
	var user *WebUser
	if head == "s" {
		var ok bool
		r.URL.Path = tail
		user, ok = securityFrontend.handleTelegramTokenAuthentication(w, r)
		if !ok {
			return
		}
	} else if head == "album" {
		var ok bool
		user, ok = securityFrontend.handleOidcAuthentication(w, r)
		if !ok {
			return
		}
	} else {
		user = &WebUser{}
	}

	log.Printf("[%s] %s %s", user, r.Method, r.URL.Path)

	// Respect the user's choice about trailing slash
	if strings.HasSuffix(originalPath, "/") && !strings.HasSuffix(r.URL.Path, "/") {
		r.URL.Path = r.URL.Path + "/"
	}

	securityFrontend.Protected.ServeHTTP(w, r)
}

func (securityFrontend *SecurityFrontend) handleOidcRedirect(w http.ResponseWriter, r *http.Request, session *sessions.Session, forcedTargetPath string) {
	nonce, err := newRandomSecret(32)
	if err != nil {
		log.Printf("rand.Read: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	state, err := newRandomSecret(32)
	if err != nil {
		log.Printf("rand.Read: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	session.AddFlash(nonce.String())
	session.AddFlash(state.String())
	if forcedTargetPath != "" {
		session.AddFlash(forcedTargetPath)
	} else {
		session.AddFlash(r.URL.Path)
	}

	err = session.Save(r, w)
	if err != nil {
		log.Printf("Session.Save: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, securityFrontend.oAuth2Config.AuthCodeURL(state.Hashed(), oidc.Nonce(nonce.Hashed())), http.StatusFound)
}

func (securityFrontend *SecurityFrontend) handleOidcCallback(w http.ResponseWriter, r *http.Request) {
	session, err := securityFrontend.store.Get(r, "oidc")
	if err != nil {
		log.Printf("session.Store.Get: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Retrieve the nonce and state from the session flashes
	nonceAndState := session.Flashes()
	if len(nonceAndState) < 3 { // there may be more than two if the user performs multiple attempts
		log.Printf("session.Flashes: no (nonce,state,redirect_path) found in current session (len = %d)", len(nonceAndState))
		securityFrontend.handleOidcRedirect(w, r, session, "/")
		return
	}

	nonce, err := secretFromHex(nonceAndState[len(nonceAndState)-3].(string))
	if err != nil {
		log.Printf("hex.DecodeString: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	state, err := secretFromHex(nonceAndState[len(nonceAndState)-2].(string))
	if err != nil {
		log.Printf("hex.DecodeString: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	redirect_path := nonceAndState[len(nonceAndState)-1].(string)
	if redirect_path == "" {
		redirect_path = "/"
	}

	if r.URL.Query().Get("state") != state.Hashed() {
		log.Println("OIDC callback: state do not match")
		http.Error(w, "state does not match", http.StatusBadRequest)
		return
	}

	oauth2Token, err := securityFrontend.oAuth2Config.Exchange(context.TODO(), r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("oauth2.Config.Exchange: %s", err)
		http.Error(w, "Invalid Authorization Code", http.StatusBadRequest)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		log.Println("Token.Extra: No id_token field in oauth2 token")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := securityFrontend.validateIdToken(rawIDToken, nonce.Hashed())
	if err != nil {
		log.Printf("validateIdToken: %s", err)
		//log.Printf("invalid id_token: %s", rawIDToken)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("HTTP: user %s logged in", user.Username)
	session.Values["user"] = &user
	err = session.Save(r, w)
	if err != nil {
		log.Printf("Session.Save: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirect_path, http.StatusFound)
}

func (securityFrontend *SecurityFrontend) validateIdToken(rawIDToken string, nonce string) (WebUser, error) {
	idToken, err := securityFrontend.oidcVerifier.Verify(context.TODO(), rawIDToken)
	if err != nil {
		return WebUser{}, fmt.Errorf("IDTokenVerifier.Verify: %s", err)
	}

	if idToken.Nonce != nonce {
		return WebUser{}, fmt.Errorf("nonces do not match in id_token")
	}

	var claims struct {
		Email        string `json:"email"`
		GSuiteDomain string `json:"hd"`
	}

	err = idToken.Claims(&claims)
	if err != nil {
		return WebUser{}, fmt.Errorf("IdToken.Claims: %s", err)
	}

	if securityFrontend.OpenId.GSuiteDomain != "" && securityFrontend.OpenId.GSuiteDomain != claims.GSuiteDomain {
		return WebUser{}, fmt.Errorf("GSuite domain '%s' is not allowed", claims.GSuiteDomain)
	}

	return WebUser{Username: claims.Email, Type: TypeOidcUser}, nil
}

func (securityFrontend *SecurityFrontend) handleOidcAuthentication(w http.ResponseWriter, r *http.Request) (*WebUser, bool) {
	session, err := securityFrontend.store.Get(r, "oidc")
	if err != nil {
		log.Printf("session.Store.Get: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return &WebUser{}, false
	}

	u := session.Values["user"]
	if u == nil {
		securityFrontend.handleOidcRedirect(w, r, session, "")
		return &WebUser{}, false
	}

	user, ok := u.(*WebUser)
	if !ok {
		log.Println("Cannot cast session item 'user' as WebUser")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return &WebUser{}, false
	}

	return user, true
}

func (securityFrontend *SecurityFrontend) handleTelegramTokenAuthentication(w http.ResponseWriter, r *http.Request) (*WebUser, bool) {
	var username, token string
	username, r.URL.Path = ShiftPath(r.URL.Path)
	token, r.URL.Path = ShiftPath(r.URL.Path)
	var tail string
	_, tail = ShiftPath(r.URL.Path)
	album, _ := ShiftPath(tail)

	data := TokenData{
		Username:    username,
		Timestamp:   time.Now(),
		Entitlement: album,
	}
	// try to validate the token with an album entitlement
	ok, err := securityFrontend.TokenGenerator.ValidateToken(data, token, securityFrontend.PerAlbumTokenValidity)
	if err != nil {
		http.Error(w, "Invalid Token", http.StatusBadRequest)
		return nil, false
	}

	if !ok {
		// if it fails, it may be a global token
		data.Entitlement = ""
		ok, err := securityFrontend.TokenGenerator.ValidateToken(data, token, securityFrontend.GlobalTokenValidity)
		if !ok || err != nil {
			http.Error(w, "Invalid Token", http.StatusBadRequest)
			return nil, false
		}
	}

	return &WebUser{Username: username, Type: TypeTelegramUser}, true
}
