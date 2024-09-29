var player = fluidPlayer('player', {
    hls: {
        overrideNative: true
    },
    layoutControls: {
        title: "TITLE PLACEHOLDER",
        doubleclickFullscreen: false,
        subtitlesEnabled: true,
        controlBar: {
            autoHide: true,
            autoHideTimeout: 2.5,
            animated: true,
            playbackRates: ['x2', 'x1.5', 'x1', 'x0.5']
        },
        miniPlayer: {
            enabled: false,
            width: 400,
            height: 225
        }
    }
});
player.setDebug(true)

video = document.getElementById("player");
vidSource = document.querySelector("source");

input_hls_url = document.getElementById("input_hls_url");
input_mp4_url = document.getElementById("input_mp4_url");
name_field = document.getElementById("user_name");

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// endpoint should be prefixed with slash
function newPost(endpoint) {
    if (!endpoint.startsWith("/")) {
        endpoint = "/" + endpoint
    }
    let req = new XMLHttpRequest();
    req.open("POST", endpoint, true);
    req.setRequestHeader('Content-Type', 'application/json');
    return req;
}

async function sendSyncEventAsync(request) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        timestamp: video.currentTime,
        username: name_field.value
    }));
}

async function sendSetAsync(request, url) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        url: url
    }));
}

function stopButton(fromHtml) {
    let request = newPost("/watch/pause")
    sendSyncEventAsync(request).then(function(res) {
        console.log("Sending stop ", res);
    });
    if (fromHtml) {
        player.pause();
    }
}

function startButton(fromHtml) {
    let request = newPost("/watch/start")
    sendSyncEventAsync(request).then(function(res) {
        console.log("Sending start ", res);
    });
    if (fromHtml) {
        player.play();
    }
}

function setHlsButton() {
    let request = newPost("/watch/set/hls")
    console.log("Current video source url: ", input_hls_url.value)
    sendSetAsync(request, input_hls_url.value).then(function(res) {
        console.log("Sending set for this hls file: ", res);
    });
}

function setMp4Button() {
    let request = newPost("/watch/set/mp4")
    console.log("Current video source url: ", input_mp4_url.value)
    sendSetAsync(request, input_mp4_url.value).then(function(res) {
        console.log("Sending set for this mp4 file: ", res);
    });
}

class AtomicBoolean {
    constructor(bool) {
        this.byte = new Int8Array(new SharedArrayBuffer(4));
        Atomics.store(this.byte, 0, bool ? 1 : 0);
    }

    setBoolean(flag) {
        Atomics.store(this.byte, 0, flag ? 1 : 0);
    }

    getBoolean() {
        return Atomics.load(this.byte, 0) === 1;
    }
}

function main() {
    let eventSource = new EventSource("/watch/events");

    let DELTA = 1.5;
    eventSource.addEventListener("start", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("Video state: PLAYING, " +
            "Priority:", jsonData["priority"],
            "Timestamp:", timestamp,
            "Origin:", jsonData["origin"]
        );
        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }
        player.play()
    })
    eventSource.addEventListener("pause", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("Video state: PAUSED, " +
            "Priority:", jsonData["priority"],
            "Timestamp:", timestamp,
            "Origin:", jsonData["origin"]
        );
        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }
        player.pause()
    })

    eventSource.addEventListener("set/hls", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Hls url recieved from the server: ", url)

        // NOTE: HLS doesn't work when source is set to a mp4 file.
        player.pause();
        vidSource.src = url;
        let hls = player.hlsInstance()
        hls.loadSource(url);
    })

    eventSource.addEventListener("set/mp4", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Mp4 url recieved from the server: ", url)

        video.pause();

        // var new_source = document.createElement('source');
        // new_source.setAttribute('src', url);
        // new_source.setAttribute('type', 'video/mp4');
        // video.appendChild(new_source);
        // video.play();

        vidSource.setAttribute('src', url);
        vidSource.setAttribute('type', 'video/mp4');
        video.load();
        video.play();
    })

    player.on('play', function() {
        startButton(false)
    });

    player.on('pause', function() {
        stopButton(false)
    });

    player.on('seeked', function(){
        console.log("seeked, currentTime", video.currentTime);
        let request= newPost("/watch/seek")
        sendSyncEventAsync(request).then(function(res) {
            console.log("Sending seek ", res);
        });
    });

    eventSource.onmessage = function(event) {
        console.log("event.data: ", event.lastEventId);
        console.log("event.data: ", event.data);
    };

}

main();
