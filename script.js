var fluidPlayer = fluidPlayer('player', {
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

video = document.getElementById("player");

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

async function sendAsync(request) {
    request.send(JSON.stringify({
        uuid: "4613443434343"
    }));
}

function stopButton() {
    let request= newPost("/watch/stop")
    sendAsync(request).then(function(res) {
        console.log(res);
    });
    video.pause();
}

function startButton() {
    let request= newPost("/watch/start")
    sendAsync(request).then(function(res) {
        console.log(res);
    });
    video.play();
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
