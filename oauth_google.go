package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

const userTokenFname = "gopro-uploader.json"

// Creates a client which can be used for Google API calls.
// Performs 3-legged OAuth2 to authorize user requests.
func newGoogleOAuth2Client(ctx context.Context, scopes ...string) (option.ClientOption, error) {
	client_secrets_path, ok := os.LookupEnv("GOOGLE_CLIENT_SECRETS")
	if !ok {
		client_secrets_path = "client_secrets.json"
	}
	client_secrets, err := ioutil.ReadFile(client_secrets_path)
	if err != nil {
		return nil, fmt.Errorf("Cannot read OAuth2 client secrets file (set GOOGLE_CLIENT_SECRETS to override path): %v", err)
	}
	config, err := google.ConfigFromJSON(client_secrets, scopes...)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse OAuth2 client secret file: %v", err)
	}
	token, err := getToken(config)
	if err != nil {
		return nil, err
	}
	return option.WithTokenSource(config.TokenSource(ctx, token)), nil
}

// Retrieves a cached OAuth2 token, refreshing it if needed.
func getToken(config *oauth2.Config) (*oauth2.Token, error) {
	tokenCacheDir, err := createTokenCacheDir()
	if err != nil {
		return nil, err
	}
	tokenCacheFile := filepath.Join(tokenCacheDir, userTokenFname)
	tok, err := getTokenFromFile(tokenCacheFile)
	if err != nil {
		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, err
		}
		err = saveToken(tokenCacheFile, tok)
		if err != nil {
			return nil, err
		}
	}
	return tok, nil
}

// Performs OAuth2 flow with Google and retrieves a token.
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\nCode: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("Unable to read authorization code %v", err)
	}
	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve token from web %v", err)
	}
	return tok, nil
}

// Reads a token from a given file path.
func getTokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// Creates and returns credential cache directory.
func createTokenCacheDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("Unable to determine user home directory. %v", err)
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return tokenCacheDir, nil
}

// Writes token to given file path.
func saveToken(file string, token *oauth2.Token) error {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
	return nil
}
