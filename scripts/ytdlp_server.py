import os
import sys
import venv
import time
import json
import http.server
import argparse
import pathlib
import subprocess
import importlib.util
import threading

def install_ytdlp(venv_dir="build/YtDlp"):
    venv_path = pathlib.Path(venv_dir)
    if not venv_path.exists():
        print("Creating yt-dlp virtual environment...")
        venv.EnvBuilder(with_pip=True).create(venv_dir)

    if os.name == "nt": # Windows
        pip_exe = venv_path / "Scripts" / "pip.exe"
        site_packages_root = venv_path / "Lib" / "site-packages"
    else: # UNIX
        pip_exe = venv_path / "bin" / "pip"
        site_packages_root = next((venv_path / "lib").glob("python*/site-packages"))

    print("Installing/upgrading yt-dlp inside venv...")
    subprocess.check_call([str(pip_exe), "install", "-U", "--pre", "yt-dlp[default,curl-cffi]"])
    subprocess.check_call([str(pip_exe), "install", "-U", "deno"])

    yt_dlp_path = site_packages_root / "yt_dlp" / "__init__.py"

    if not yt_dlp_path.exists():
        raise RuntimeError("yt_dlp did not install correctly inside the venv.")

    spec = importlib.util.spec_from_file_location("yt_dlp", yt_dlp_path)
    yt_dlp = importlib.util.module_from_spec(spec)
    sys.modules["yt_dlp"] = yt_dlp
    spec.loader.exec_module(yt_dlp)
    return yt_dlp

def autoupdate_ytdlp():
    def loop():
        global yt_dlp
        while True:
            time.sleep(24 * 60 * 60)
            print("Auto-upgrading YtDlp library")
            yt_dlp = install_ytdlp()
    threading.Thread(target=loop, daemon=True).start()

def setup_env(venv_dir="build/YtDlp"):
    venv_path = pathlib.Path(venv_dir)
    if not venv_path.exists():
        return

    if os.name == "nt":
        bindir = venv_path / "Scripts"
    else:
        bindir = venv_path / "bin"

    os.environ["VIRTUAL_ENV"] = str(venv_path)
    os.environ["PATH"] = str(bindir) + os.pathsep + os.environ.get("PATH", "")
    os.environ.pop("PYTHONHOME", None)

    site_packages_root = venv_path / "lib"
    matches = list(site_packages_root.glob("python*/site-packages"))
    if matches:
        site_packages = matches[0]
        if str(site_packages) not in sys.path:
            sys.path.insert(0, str(site_packages))

    sys.prefix = str(venv_path)

# Bootstrapping YtDlp library
yt_dlp = install_ytdlp()
setup_env()

class YoutubeVideo:
    def __init__(self, 
                 id: str, title: str, thumbnail: str, original_url: str, manifest_url: str, 
                 available_at: int, duration: int, upload_date: str, uploader: str, 
                 artist: str, album: str, release_date: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.manifest_url = manifest_url
        self.available_at = available_at
        self.duration     = duration
        self.upload_date  = upload_date
        self.uploader     = uploader

        # NOTE(kihau): Extra metadata for tracks from YouTube Music. Might not exists, depending on the video.
        self.artist       = artist
        self.album        = album
        self.release_date = release_date


class YoutubePlaylistVideo:
    def __init__(self, url: str, title: str, thumbnails: list):
        self.url          = url
        self.title        = title
        self.thumbnails   = thumbnails

class YoutubePlaylist:
    def __init__(self, entries: list[YoutubePlaylistVideo]):
        self.entries = entries

def get_youtube_playlist(query: str, start: int, end: int):
    ytplaylist_opts = {
        "remote_components": ["ejs:github"],
        'extract_flat': True,
        'playliststart': start + 1,
        'playlistend':   end,
        'color': 'no_color',
        'dump_single_json': True,
    }

    ytplaylist = yt_dlp.YoutubeDL(ytplaylist_opts)
    info = ytplaylist.extract_info(query, download=False)
    videos = []

    if info is None:
        raise Exception("Yt-Dlp output data is missing")

    entries = info.get('entries')
    if entries is None:
        # HACK: Try to refetch the playlist
        playlist_url = info.get("url")
        if playlist_url is None:
            return YoutubePlaylist([])

        info = ytplaylist.extract_info(playlist_url, download=False)
        if info is None:
            raise Exception("Yt-Dlp output data is missing")

    entries = info.get('entries')
    if entries is None:
        return YoutubePlaylist([])

    for entry in entries:
        url        = entry["url"]
        title      = entry["title"]
        thumbnails = entry["thumbnails"]
        video      = YoutubePlaylistVideo(url, title, thumbnails)
        videos.append(video)

    return YoutubePlaylist(videos)

def bench(note, func):
    start = time.time()
    data = func()
    end = time.time()
    print(f"[BENCH] {note} elapsed time: " + str(end - start))
    return data

def get_youtube_video(query: str):
    from yt_dlp.utils import (
        DownloadError,
        GeoRestrictedError,
        UnavailableVideoError,
    )

    ytfetch_opts = {
        # NOTE(kihau): Only request videos with either H264 or H265 codec.
        # "format": "(bv*[vcodec~=\"^((he|a)vc|h26[45])\"]+ba)",
        
        "remote_components": ["ejs:github"],
        "extractor_args": {
            "youtube": {
                "player_client": ["web_safari"],
            }
        },
        "playlist_items": "1",
        "noplaylist": True,
        "color": "no_color",
    }

    ytfetch = yt_dlp.YoutubeDL(ytfetch_opts)

    try:
        info = bench("extract_info", lambda : ytfetch.extract_info(query, download=False))
    except DownloadError as error:
        if error.msg is None:
            return None, "Failed to find YouTube video." 
        else:
            items = error.msg.split(":")[2:]
            error_message = ":".join(items)
            return None, error_message

    except UnavailableVideoError as error:
        return None, "YouTube video is not available." 

    except GeoRestrictedError as error:
        return None, "Failed to load YouTube video due to geo-restriction." 

    except Exception as exception:
        return None, str(exception)

    if info is None:
        return None, "Failed to find YouTube video." 

    entries = info.get("entries")
    entry   = None

    if entries is None:
        entry = info
    elif len(entries) != 0:
        entry = entries[0]

    if entry is None:
        return None, "No YouTube videos found."

    id           = entry.get("id")
    title        = entry.get("title")
    thumbnail    = entry.get("thumbnail")
    original_url = entry.get("original_url")
    manifest_url = entry.get("manifest_url")
    available_at = entry.get("available_at")
    duration     = entry.get("duration")
    upload_date  = entry.get("upload_date")
    uploader     = entry.get("uploader")

    # NOTE(kihau): YouTube Music metadata
    artist       = entry.get("artist")
    album        = entry.get("album")
    release_date = entry.get("release_date")

    if not isinstance(id, str): 
        id = ""

    if not isinstance(title, str):
        title = "Video title is missing :("

    if not isinstance(thumbnail, str):
        thumbnail = ""

    if not isinstance(original_url, str):
        original_url = ""

    if not isinstance(manifest_url, str):
        return None, "Failed to fetch YouTube video. Source URL is missing."

    if not isinstance(available_at, int):
        available_at = 0

    if not isinstance(duration, int):
        duration = 0

    if not isinstance(upload_date, str):
        upload_date = ""

    if not isinstance(uploader, str):
        uploader = ""

    if not isinstance(artist, str):
        artist = ""

    if not isinstance(album, str):
        album = ""

    if not isinstance(release_date, str):
        release_date = ""

    ytVideo = YoutubeVideo(
        id           = id,
        title        = title,
        thumbnail    = thumbnail,
        original_url = original_url,
        manifest_url = manifest_url,
        available_at = available_at,
        duration     = duration,
        upload_date  = upload_date,
        uploader     = uploader,
        artist       = artist,
        album        = album,
        release_date = release_date,
    )

    return ytVideo, None

class TwitchStream:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, url: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.url          = url

class TikTok:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, url: str, path: str):
        self.id = id
        self.title = title
        self.thumbnail = thumbnail
        self.original_url = original_url
        self.url = url
        self.path = path

def download_tiktok_video(url: str):
    ts = str(int(time.time()))
    outtmpl = f'content/media/video/{ts}.%(ext)s'
    tiktok_opts = {
        'outtmpl': outtmpl,
        'quiet' : False,
        'format': 'best',             
    }

    tiktok = yt_dlp.YoutubeDL(tiktok_opts)
    print("Current URL:", url)
    tiktok.download([url])
    info = bench("extract_info", lambda : tiktok.extract_info(url, download=True))

    if info is None:
        raise Exception("Yt-Dlp did not return any TikTok video")
    
    id           = info.get("id")
    uploader     = info.get("uploader")
    description  = info.get("description")
    thumbnail    = info.get("thumbnail")
    title        = info.get("title")

    if uploader is None: 
        uploader = "Unknown"

    if not isinstance(description, str): 
        description = "[Tiktok title is missing]"

    if thumbnail is None: 
        thumbnail = ""
    
    return TikTok(id, title, thumbnail, "", url, "content/media/video/" + ts)

def get_twitch_stream(url: str):
    twitch_opts = { 
        'noplaylist': True,
        'color': 'no_color',
    }
    twitch = yt_dlp.YoutubeDL(twitch_opts)
    info = bench("extract_info", lambda : twitch.extract_info(url, download=False))

    if info is None:
        raise Exception("Yt-Dlp did not returned any Twitch streams")

    id           = info.get("id")
    uploader     = info.get("uploader")
    description  = info.get("description")
    thumbnail    = info.get("thumbnail")
    original_url = info.get("original_url")
    source_url   = info.get("url")

    if uploader is None: 
        uploader = "Unknown"

    if not isinstance(description, str): 
        description = "[Stream title is missing]"

    if thumbnail is None: 
        thumbnail = ""

    if not isinstance(original_url, str): 
        original_url = ""

    if source_url is None: 
        source_url = ""

    title = f"Twitch {uploader} (live) - {description}"
    return TwitchStream(id, title, thumbnail, original_url, source_url)

class TwitterFormat:
    def __init__(self, manifest_url: str):
        self.manifest_url = manifest_url

class TwitterSource:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, formats: list[TwitterFormat], duration: int):
        self.id = id
        self.title = title
        self.thumbnail = thumbnail
        self.original_url = original_url
        self.formats = formats
        self.duration = duration

def get_twitter_source(url: str):
    twitter_opts = { 
        'noplaylist': True,
        'color': 'no_color',
    }
    twitch = yt_dlp.YoutubeDL(twitter_opts)
    info = bench("extract_info", lambda : twitch.extract_info(url, download=False))

    if info is None:
        raise Exception("Yt-Dlp did not returned any Twitch streams")

    id           = info.get("id")
    title        = info.get("title")
    thumbnail    = info.get("thumbnail")
    original_url = info.get("original_url")
    formats      = info.get("formats")
    duration     = info.get("duration")

    if title is None: 
        title = "Twitter video " + id

    if thumbnail is None: 
        thumbnail = ""

    if not isinstance(original_url, str): 
        original_url = ""

    twitter_formats = []
    if formats is not None:
        for format in formats:
            manifest_url = format.get("manifest_url")
            if isinstance(manifest_url, str) and manifest_url != "": 
                twitter_formats.append(TwitterFormat(manifest_url))

    if duration is None: 
        duration = 0

    return TwitterSource(id, title, thumbnail, original_url, twitter_formats, duration)

class YtdlpServer(http.server.BaseHTTPRequestHandler):
    def handle_youtube_fetch(self, query):
        output, errorMessage = bench('get_youtube_video', lambda : get_youtube_video(query))

        if output is not None:
            self.send_response(200)
            data = output
        else:
            self.send_response(503)
            data = errorMessage

        jsondata = json.dumps(data, default=vars)
        response = bytes(jsondata, "utf-8")

        self.send_header("Content-type", "application/json")
        self.end_headers()

        self.wfile.write(response)

    def handle_youtube_playlist(self, query, start, max):
        output = bench('get_youtube_playlist', lambda : get_youtube_playlist(query, start, max))
        self.send_response(200)
        self.send_header("Content-type", "application/json")
        self.end_headers()

        jsondata = json.dumps(output, default=vars)
        response = bytes(jsondata, "utf-8")
        self.wfile.write(response)

    def handle_request(self):
        data = json.loads(self.rfile.read1())

        if self.path == '/youtube/fetch':
            self.handle_youtube_fetch(data["query"])

        elif self.path == '/youtube/search':
            count = data["count"]
            query = f'ytsearch{count}:' + data["query"]
            self.handle_youtube_playlist(query, 0, count)

        elif self.path == '/youtube/playlist':
            self.handle_youtube_playlist(data["query"], data["start"], data["end"])

        elif self.path == '/twitch/fetch':
            output = bench('get_twitch_stream', lambda : get_twitch_stream(data))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(output, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        elif self.path == '/twitter/fetch':
            output = bench('get_twitter_source', lambda : get_twitter_source(data))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(output, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        elif self.path == '/tiktok/fetch':
            output = bench('download tiktok video', lambda : download_tiktok_video(data))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(output, default=vars)
            print("Json data:",jsondata)
            response = bytes(jsondata, "utf-8")
            print("Response:", response)
            self.wfile.write(response)
            
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        try:
            self.handle_request()

        except Exception as exception:
            self.send_response(503)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            exception_string = str(exception)
            jsondata = json.dumps(exception_string, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

            raise exception
    
    def do_GET(self):
        print(f"--- Processing Request for: {self.path} ---")
        self.send_response(200)
        self.end_headers()


def main():
    autoupdate_ytdlp()

    parser = argparse.ArgumentParser(
        prog="YtdlpServer",
        description="Internal Ytdlp Python Server",
    )

    parser.add_argument('-cp', '--config-path', type=str, help="Path to the config json.")
    args = parser.parse_args()

    hostname = "localhost"
    port = 2345

    if args.config_path is not None:
        with open(args.config_path, 'r') as file:
            data     = json.load(file)
            config = data.get("ytdlp")
            if config is None:
                print("Ytdlp internal server config not found in specified config json file.")
                return

            enabled       = config.get("enabled")
            enable_server = config.get("enable_server")
            if enabled is False or enable_server is False:
                print("Ytdlp internal server is disabled as specified in the config file.")
                return

            hostname = config.get("server_address")
            port     = config.get("server_port")

    web_server = http.server.ThreadingHTTPServer((hostname, port), YtdlpServer)
    print("Running an ytdlp helper server at http://%s:%s" % (hostname, port))

    web_server.serve_forever()

main()
