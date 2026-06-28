# 🎨 NISABA — UI/UX Mockup Prompts for Draw Things (Updated)

These prompts are designed for high-fidelity UI/UX generation to guide the development of the **NISABA** (gamerepo) frontend using **TailwindCSS** and **HTMX**.

---

### 1. Main Library Dashboard (Grid View)
**Prompt**: A high-fidelity UI/UX mockup of a modern game library dashboard, dark mode, glassmorphism. Central dense grid of portrait game covers with rounded corners. Each cover has subtle bottom-right badges for Steam Deck Verified (green checkmark) and Proton Platinum rating, and top-right small icons indicating store ownership (Steam, GOG, Epic logos). A sleek sidebar on the left with categories: "All Games", "Playing", "Installed", "Wishlist". Top search bar with integrated filters for "Genre", "Developer", and "Device: Steam Deck". Deep charcoal and midnight blue color palette with vibrant accent highlights. Professional, clean, 8k resolution, trending on Dribbble.

### 2. Game Detail "Hero" View (Enhanced)
**Prompt**: UI design for a game detail page in a desktop app. Large cinematic background hero image with a soft bottom-fade. Centered transparent game logo at the top. Two-column layout: left column shows metadata (Developer, Publisher, Release Date, Play Time) and a list of expandable DLC contents, right column shows "Install Status" with a structured list of devices and volumes (e.g., "Praxis - Internal SSD", "Ergaster - D: Drive"). A prominent "PLAY" button in emerald green next to a "SYNC" button. Star rating widget (4.5/5 stars) and a section for "Personal Notes". Minimalist typography, San Francisco font, high contrast.

### 3. Wishlist & "Instant Buy" Deals View
**Prompt**: User interface for a game price tracking and wishlist dashboard. List view showing game titles, current price, and historical low. Color-coded indicators: "Instant Buy" threshold in neon green, "Consider" threshold in amber yellow. Small sparkline charts showing price history from IsThereAnyDeal. Store icons (Steam, GOG, Epic) next to each price. Modern, clean, data-heavy but readable. Dark mode with subtle gradients. Cyberpunk-adjacent aesthetic but functional and professional.

### 4. Sync & Pipeline Progress Interface
**Prompt**: Technical dashboard UI for an automated metadata enrichment pipeline. A "Sync All" button at the top. Below, a series of cards for each store (Steam, GOG, Epic, Amazon) with circular progress loaders and status labels like "Syncing...", "Matched", "Needs Review". A live "Activity Feed" showing recent acquisitions with small thumbnails. Modern dashboard aesthetic, similar to Vercel or Stripe, using TailwindCSS styling principles. Clean lines, monospace fonts for technical data, high-density information layout.

### 5. Mobile/Handheld "Companion" View
**Prompt**: Mobile UI mockup for a game library manager on a handheld screen. Portrait layout. Top section shows "Device Status" (Storage used/available). Below, a "Continue Playing" horizontal scroll of game covers. Large touch-friendly buttons for "Quick Sync" and "Add to Wishlist". Bottom navigation bar with icons for Library, Search, and Settings. Designed for a 16:10 aspect ratio (Steam Deck style). High-fidelity, modern mobile app design, dark theme, vibrant highlights.

---

### 💡 Suggested Draw Things Settings:
*   **Model**: SDXL or a high-quality UI-tuned model (e.g., *RealVisXL* or *Juggernaut XL*).
*   **Negative Prompt**: *Blurry, low resolution, messy text, crowded, outdated, Windows 95 style, neon overload, unreadable, ugly icons, watermark.*
*   **Aspect Ratio**: 16:9 (Desktop) or 16:10 (Handheld/Steam Deck).
*   **Sampling Method**: DPM++ 2M SDE Karras or similar.