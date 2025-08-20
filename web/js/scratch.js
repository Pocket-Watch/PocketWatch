import {Options, Player} from "./custom_player.js"

let player = null;

function setOnClick() {
    let input_url = document.getElementById("input_url");
    player.setVideoTrack(input_url.value);
}

function destroy() {
    player.destroyPlayer();
}

function attach() {
    main();
}

function main() {
    window.setOnClick = setOnClick;
    window.attach = attach;
    window.destroy = destroy;

    let video0 = document.getElementById("video0");
    let options = new Options();
    options.hideNextButton = true;
    //options.useAudioGain = true;
    player = new Player(video0, options);

    //let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    //let track = "https://test-streams.mux.dev/x36xhzz/url_6/193039199_mp4_h264_aac_hq_7.m3u8"
    let track = "https://video.blender.org/static/web-videos/264ff760-803e-430e-8d81-15648e904183-720.mp4"
    // let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    player.setVideoTrack(track);
    player.setTitle("Agent327");
    player.addSubtitle("media/subs/Tears.of.Steel.2012.vtt")
    player.addSubtitle("media/subs/Agent327.srt")
    player.addSubtitle("media/subs/oneline.srt")
    player.setVolume(0.01)
    player.onControlsPlay(() => {
        player.setToast("User clicked play.");
    })

    player.onControlsPause(() => {
        player.setToast("User clicked pause.");
    })

    player.onControlsSeeked(timestamp => {
        player.setToast("User seeked to " + timestamp.toFixed(3));
    })

    player.onSettingsChange((key, value) => {
        console.log("Settings change:", key, "to", value);
    })

    player.onSubtitleSearch(async (search) => {
        console.log("Search requested.", search);
        return true;
    })

    player.onPlaybackError(function (exception, error) {
        console.log(exception.name, "-", exception.message);
        if (!error) {
            return;
        }
        // https://developer.mozilla.org/en-US/docs/Web/API/MediaError
        console.log(error.code, "-", error.message);
    })
}

main();
