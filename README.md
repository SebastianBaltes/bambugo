# BambuGo 🐼🌕

**BambuGo** is a lightweight, self-hosted bridge and dashboard for Bambu Lab printers (P1S, P1P, X1C). It solves the common stability issues with the official Bambu Network plugin in Orca Slicer by bridging the printer to a standard **Moonraker/Klipper API**.

Additionally, it provides a beautiful, mobile-first React dashboard to monitor and control your printer from anywhere (especially useful when combined with Tailscale).

## Features

- 🚀 **Moonraker Bridge:** Use your Bambu printer in Orca Slicer via the stable "Klipper" connection type.
- 📱 **Mobile Dashboard:** A responsive React UI for monitoring temperatures, progress, and AMS status.
- 📹 **Live Camera:** Integrated support for the RTSPS camera stream (via `go2rtc`), bypassing the proprietary MJPEG port.
- 💡 **Remote Control:** Toggle chamber lights, pause, resume, or stop prints directly from the web.
- 📁 **File Management:** Upload `.gcode.3mf` files via FTPS and start prints from the browser.
- 🛡️ **Tailscale Ready:** Designed to work perfectly with Tailscale for secure remote access without port forwarding.

## Project Structure

- `backend/`: A high-performance Go service handling MQTT, FTPS, and Moonraker API emulation.
- `frontend/`: A modern Vite + React application optimized for mobile devices.

## Installation

### Prerequisites

- A Raspberry Pi or any Linux server.
- [Go](https://golang.org/) (v1.22+).
- [Node.js](https://nodejs.org/) and npm.
- [go2rtc](https://github.com/AlexxIT/go2rtc) for camera streaming.

### Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/SebastianBaltes/bambugo.git
   cd bambugo
   ```

2. **Configure the printer:**
   Update the constants in `backend/main.go` (IP, Serial Number, and Access Code). *Note: We plan to move these to a config file soon.*

3. **Build and run the Backend:**
   ```bash
   cd backend
   go build -o main main.go
   ./main
   ```

4. **Run the Frontend:**
   ```bash
   cd frontend
   npm install
   npm run dev -- --host
   ```

## Orca Slicer Integration

Say goodbye to Bambu Network Plugin crashes!

1. Open **Orca Slicer**.
2. Go to your **Printer Settings**.
3. In the **Advanced** section, check **"Use 3rd-party print host"**.
4. Click the **WLAN/WiFi icon** next to the printer name.
5. Set **Host Type** to **"Octo/Klipper"**.
6. Set **Hostname** to your Pi's IP (e.g., `http://192.168.178.51:8080`).
7. Click **Test** to confirm the connection.

## License

This project is licensed under the **MIT License** - see the [LICENSE](LICENSE) file for details.

---
*Created with love by Monsterpi for Sebastian Baltes.*
