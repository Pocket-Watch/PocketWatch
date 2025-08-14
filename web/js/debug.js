function createEntry(title, url) {
    const requestEntry = {
        url:          url,
        title:        title,
        use_proxy:    false,
        referer_url:  "",
        subtitles:    [],
        search_video: false,
        is_playlist:  false,
        playlist_skip_count: 0,
        playlist_max_size:   0,
    };

    return requestEntry;
}

async function createExamplePlaylist(count = 10) {
    let api = await import("./api.js");

    for (let i = 0; i <= count; i++) {
        let entry = createEntry(`Example ${i}`, `https://example.com/video${i}.mp4`);
        await api.playlistAdd(entry);
    }
}

async function setExampleEntry() {
    let api = await import("./api.js");

    let subtitle = {
        id:    0,
        name:  "Big Buck Bunny",
        url:   "media/subs/sample.srt",
        shift: 0.0,
    };

    const requestEntry = {
        url:          "media/video/big_buck_bunny.mp4",
        title:        "Big Buck Bunny",
        use_proxy:    false,
        referer_url:  "",
        search_video: false,
        is_playlist:  false,
        add_to_top:   false,
        subtitles:    [ subtitle ],
        playlist_skip_count: 0,
        playlist_max_size:   20,
    };

    api.playerSet(requestEntry);
}

async function setExampleProxy() {
    let api = await import("./api.js");

    let subtitle = {
        id:    0,
        name:  "Big Buck Bunny",
        url:   "media/subs/sample.srt",
        shift: 0.0,
    };

    const requestEntry = {
        url:          "https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8",
        title:        "Big Buck Bunny Hls + Proxy",
        use_proxy:    true,
        referer_url:  "",
        search_video: false,
        is_playlist:  false,
        add_to_top:   false,
        subtitles:    [ subtitle ],
        playlist_skip_count: 0,
        playlist_max_size:   20,
    };

    api.playerSet(requestEntry);
}

function importAll(module) {
    let entries = Object.entries(module);

    for (let i = 0; i < entries.length; i++) {
        const entry = entries[i];
        const name   = entry[0];
        window[name] = entry[1];
    }
}

import("./util.js").then(mod => importAll(mod));
import("./api.js").then(mod => window.api = mod);
import("./main.js").then(mod => window.main = mod);
import("./playlist.js").then(mod => window.playlist = mod);
import("./chat.js").then(mod => window.chat = mod);
import("./history.js").then(mod => window.history = mod);
import("./custom_player.js").then(mod => window.player = mod);
