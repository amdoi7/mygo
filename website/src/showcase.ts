/**
 * Apps built with MyGo, shown at /showcase. To add yours, open a pull request
 * that adds an entry to the end of this list.
 *
 * - name: the app's name.
 * - description: what it does, in a sentence or two.
 * - url: where to get it or read about it, such as its repository.
 * - author: optional, your name or GitHub login.
 * - mode: optional, "native" when its windows draw native UI (package ui),
 *   "web" when they show a web frontend in the system webview.
 * - image: optional, a screenshot. Put it in website/public/showcase/ and
 *   give its path, such as "/showcase/my-app.png" (about 1600×1000, under
 *   300 KB).
 */
export interface ShowcaseApp {
  name: string
  description: string
  url: string
  author?: string
  mode?: "native" | "web"
  image?: string
}

export const showcase: ShowcaseApp[] = [
  {
    name: "MixMusic",
    description:
      "An open-source music player for macOS, Windows and Linux. One search looks through YouTube Music and Spotify, and one queue and library mix tracks from both.",
    url: "https://github.com/fzn0x/mixmusic",
    author: "fzn0x",
    mode: "web",
  },
  {
    name: "Pulse",
    description:
      "An activity monitor for Mac, Windows and Linux. Every process with its app's icon, each CPU core, memory pressure, the GPU, disks, network connections, energy and temperatures, with live readings in the menu bar.",
    url: "https://pulse.egoist.dev",
    author: "egoist",
    mode: "native",
    image: "/showcase/pulse.jpg",
  },
  {
    name: "Kero",
    description:
      "A terminal for coding agents. Sessions keep running when you quit, a sidebar shows which agents are working, which need you and which are done, and a CLI lets agents drive it, on your machine or a remote host.",
    url: "https://kero.sh",
    author: "egoist",
    mode: "native",
  },
  {
    name: "Lorca",
    description:
      "AI teammates on your computer. Chat with one bot or put several in a group chat; they read and edit files, run commands and hand work to each other, and chats sync end-to-end encrypted to your other devices.",
    url: "https://lorca.app",
    author: "egoist",
    mode: "native",
  },
  {
    name: "MyGo 云音乐",
    description:
      "A third-party NetEase Cloud Music client. Daily picks, playlists, search, personal FM, word-by-word lyrics, lossless and Hi-Res playback, podcasts with on-device transcripts, and a mini player in the notch.",
    url: "https://cloudmusic.egoist.dev",
    author: "egoist",
    mode: "native",
  },
  {
    name: "GoRex",
    description:
      "A replica of Superlogical's Rex terminal. Tabs and split panes as cards, sessions that keep running after you quit, and each pane's program and activity in its header.",
    url: "https://github.com/egoist/gorex",
    author: "egoist",
    mode: "native",
    image: "/showcase/gorex.jpg",
  },
  {
    name: "Godiff",
    description:
      "A minimal diff viewer for reviewing Git changes and committing them. Split and unified diffs, review comments to copy for an agent, history, and GitHub pull request reviews.",
    url: "https://github.com/egoist/godiff",
    author: "egoist",
    mode: "native",
  },
  {
    name: "2048",
    description:
      "The classic 2048 puzzle, played with the arrow keys, a drag or a touchpad swipe. Its WebAssembly build runs in the browser too.",
    url: "https://github.com/egoist/2048",
    author: "egoist",
    mode: "native",
    image: "/showcase/2048.jpg",
  },
  {
    name: "Uma",
    description: "A menu bar app that shows the websites of an Umami account and their visitors.",
    url: "https://github.com/egoist/uma",
    author: "egoist",
    mode: "native",
  },
]
