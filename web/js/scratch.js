import {Options, Player} from "./custom_player.js"

let player = null;

let videoInput = document.getElementById("demo_video_url");
let audioInput = document.getElementById("demo_audio_url");

function setVideoOnClick() {
    player.setVideoTrack(videoInput.value);
}

function setAudioOnClick() {
    player.setAudioTrack(audioInput.value);
}

function destroy() {
    player.destroyPlayer();
}

function attach() {
    main();
}

function main() {
    window.setVideoOnClick = setVideoOnClick;
    window.setAudioOnClick = setAudioOnClick;
    window.attach = attach;
    window.destroy = destroy;

    let video0 = document.getElementById("video0");
    let options = new Options();
    options.hideNextButton = true;
    options.iconsPath = "../svg/player_icons.svg";
    options.useVolumeGain = true;
    options.maxVolume = 1.5;
    player = new Player(video0, options);

    //let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    //let track = "https://test-streams.mux.dev/x36xhzz/url_6/193039199_mp4_h264_aac_hq_7.m3u8"
    let track = "https://video.blender.org/static/web-videos/264ff760-803e-430e-8d81-15648e904183-720.mp4";
    //let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    player.setVideoTrack(track);
    player.setTitle("Agent327");
    const subsDir = "../content/media/subs/"
    player.addSubtitle(subsDir + "Tears.of.Steel.2012.vtt", "ToS", 2);
    player.addSubtitle(subsDir + "Agent327.srt");
    player.addSubtitle(subsDir + "oneline.srt");
    player.setVolume(0.01);

    player.onControlsPlay(() => {
        player.setToast("User clicked play.");
    });

    player.onControlsPause(() => {
        player.setToast("User clicked pause.");
    });

    player.onControlsSeeked(timestamp => {
        player.setToast("User seeked to " + timestamp.toFixed(3));
    });

    player.onSettingsChange((key, value) => {
        console.log("Settings change:", key, "to", value);
    });

    player.onSubtitleSearch(async (search) => {
        console.log("Search requested.", search);
        return true;
    });

    player.onPlaybackError(function (exception, error) {
        console.log(exception.name, "-", exception.message);
        if (!error) {
            return;
        }
        // https://developer.mozilla.org/en-US/docs/Web/API/MediaError
        console.log(error.code, "-", error.message);
    })

    // Expose player for debugging
    window.player = player;
}

main();
