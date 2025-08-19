import yt_dlp
import time
import json
import http.server

class YoutubeVideo:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, manifest_url: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.manifest_url = manifest_url

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
        'extract_flat': True,
        'playliststart': start,
        'playlistend':   end,
        'color': 'no_color',
    }

    ytplaylist = yt_dlp.YoutubeDL(ytplaylist_opts)
    info = ytplaylist.extract_info(query, download=False)
    videos = []

    if info is None:
        raise Exception("Yt-Dlp output data is missing")

    entries = info['entries']
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
    ytfetch_opts = {
        # NOTE(kihau): Only request videos with either H264 or H265 codec.
        # 'format': '(bv*[vcodec~=\'^((he|a)vc|h26[45])\']+ba)',
        
        'extractor_args': {
            'youtube': {
                'player_client': ['web_safari'],
            }
        },
        'playlist_items': '1',
        'noplaylist': True,
        'color': 'no_color',
    }

    ytfetch = yt_dlp.YoutubeDL(ytfetch_opts)
    info = bench("extract_info", lambda : ytfetch.extract_info(query, download=False))

    if info is None:
        raise Exception("Yt-Dlp output data is missing")

    entries = info.get("entries")
    if entries is None or len(entries) == 0:
        entry = info
    else:
        entry = entries[0]

    if entry is None:
        raise Exception("Yt-Dlp did not return any YouTube videos")

    id           = entry["id"]
    title        = entry["title"]
    thumbnail    = entry["thumbnail"]
    original_url = entry["original_url"]
    manifest_url = entry["manifest_url"]

    return YoutubeVideo(id, title, thumbnail, original_url, manifest_url)

class TwitchStream:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, url: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.url          = url

def get_twitch_stream(url: str):
    twitch_opts = { 
        'noplaylist': True,
        'color': 'no_color',
    }
    twitch = yt_dlp.YoutubeDL(twitch_opts)
    info = bench("extract_info", lambda : twitch.extract_info(url, download=False))

    if info is None:
        raise Exception("Yt-Dlp did not returned any Twitch streams")

    id           = info["id"]
    uploader     = info["uploader"]
    description  = info["description"]
    thumbnail    = info["thumbnail"]
    original_url = info["original_url"]
    url          = info["url"]

    title        = f"Twitch {uploader} (live) - {description}"

    return TwitchStream(id, title, thumbnail, original_url, url)

class InternalServer(http.server.BaseHTTPRequestHandler):
    def handle_request(self):
        if self.path == '/youtube/fetch':
            request = json.loads(self.rfile.read1())
            data = bench('get_youtube_video', lambda : get_youtube_video(request["query"]))

            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        elif self.path == '/youtube/playlist':
            request = json.loads(self.rfile.read1())

            data = bench('get_youtube_playlist', lambda : get_youtube_playlist(request["query"], request["start"], request["end"]))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        elif self.path == '/twitch/fetch':
            url  = json.loads(self.rfile.read1())
            data = bench('get_twitch_stream', lambda : get_twitch_stream(url))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
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

hostname = "localhost"
port = 2345

web_server = http.server.ThreadingHTTPServer((hostname, port), InternalServer)
print("Running an internal helper server at http://%s:%s" % (hostname, port))

web_server.serve_forever()
