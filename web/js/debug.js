
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
