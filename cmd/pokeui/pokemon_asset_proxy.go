package main

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	pokemonAssetUpstreamRoot = "https://raw.githubusercontent.com/PokeAPI/sprites/master/"
	pokemonAssetClient       = &http.Client{Timeout: 5 * time.Second}

	pokemonSpritePath = regexp.MustCompile(`^sprites/pokemon/versions/generation-i/red-blue/[0-9]{1,3}\.png$`)
	pokemonItemPath   = regexp.MustCompile(`^sprites/items/[a-z0-9-]+\.png$`)
	pokemonBadgePath  = regexp.MustCompile(`^sprites/badges/[1-8]\.png$`)
)

func validPokemonAssetPath(name string) bool {
	return pokemonSpritePath.MatchString(name) || pokemonItemPath.MatchString(name) || pokemonBadgePath.MatchString(name)
}

func servePokemonAsset(res http.ResponseWriter, req *http.Request) {
	name := strings.TrimPrefix(req.URL.Path, "/poke-assets/")
	if name == "" || strings.Contains(name, "..") || !validPokemonAssetPath(name) {
		http.NotFound(res, req)
		return
	}

	up, err := http.NewRequestWithContext(req.Context(), http.MethodGet, pokemonAssetUpstreamRoot+name, nil)
	if err != nil {
		http.NotFound(res, req)
		return
	}
	up.Header.Set("Accept", "image/png,image/*;q=0.8")
	resp, err := pokemonAssetClient.Do(up)
	if err != nil {
		http.Error(res, "pokemon artwork unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.NotFound(res, req)
		return
	}

	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "public, max-age=86400")
	res.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(res, io.LimitReader(resp.Body, 2<<20))
}
