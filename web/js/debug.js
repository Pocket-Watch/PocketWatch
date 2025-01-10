function createEntry(title, url) {
    const entry = {
        id:           0,
        url:          url,
        title:        title,
        user_id:      0,
        use_proxy:    false,
        referer_url:  "",
        subtitle_url: "",
        created:      new Date,
    };

    return entry;
}

async function createExamplePlaylist(count = 10) {
    let api = await import("./api.js");

    for (let i = 0; i <= count; i++) {
        let entry = createEntry(`Example ${i}`, `https://example.com/video${i}.mp4`)
        await api.playlistAdd(entry);
    }
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
import("./custom_player.js").then(mod => window.player = mod);
