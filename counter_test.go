package main

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestURLGenerator(t *testing.T) {
	tests := []struct {
		name string
		arg  io.Reader
		want []string
	}{{
		name: "Regular",
		arg:  strings.NewReader("https://stackoverflow.com/questions/tagged/go?tab=Votes\nhttps://habr.com/ru/hub/go/top/alltime/\nhttps://golang.org/ref/spec"),
		want: []string{"https://stackoverflow.com/questions/tagged/go?tab=Votes", "https://habr.com/ru/hub/go/top/alltime/", "https://golang.org/ref/spec"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for link := range URLGenerator(tt.arg) {
				got = append(got, link)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("URLGenerator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntranceCount(t *testing.T) {
	type args struct {
		source      io.Reader
		desiredWord string
	}
	tests := []struct {
		name       string
		args       args
		wantAmount uint
		wantErr    bool
	}{{
		name: "regular count",
		args: args{
			source: strings.NewReader( /* language=HTML */ `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="description"
          content="Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.">
    <title>The Go Programming Language</title>
</head>
<body class="Site">
<main id="page" class="Site-content">
    <div class="container">
        <div id="nav"></div>
        <div class="HomeContainer">
            <section class="HomeSection Hero">
                <h1 class="Hero-header">
                    Go is an open source programming language that makes it easy to build
                    <strong>simple</strong>, <strong>reliable</strong>, and <strong>efficient</strong> software.
                </h1>
                <i class="Hero-gopher"></i>
                <a href="/dl/" class="Button Button--big HeroDownloadButton">
                    <img class="HeroDownloadButton-image" src="/lib/godoc/images/cloud-download.svg" alt="">
                    Download Go
                </a>
                <p class="Hero-description">
                    Binary distributions available for<br>
                    Linux, macOS, Windows, and more.
                </p>
            </section>
        </div>
    </div>
</main>
</body>
</html>`),
			desiredWord: "go",
		},
		wantAmount: 6,
		wantErr: false,
	}, {
		name: "empty count",
		args: args{
			source:      strings.NewReader(""),
			desiredWord: "go",
		},
		wantAmount: 0,
		wantErr:    false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAmount, err := EntranceCount(tt.args.source, tt.args.desiredWord)
			if (err != nil) != tt.wantErr {
				t.Errorf("EntranceCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAmount != tt.wantAmount {
				t.Errorf("EntranceCount() gotAmount = %v, want %v", gotAmount, tt.wantAmount)
			}
		})
	}
}
