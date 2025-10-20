# Kubernaut Presentation - Act 1: Customer Pain

## Target Audience
**Technical Product Managers for Cloud Services**

Focus: Market opportunity, competitive positioning, timing argument

## Files Overview

- **`slide-01-CONSOLIDATED.md`** ⭐ **USE THIS** - Optimized 5-slide version for 8-minute presentation
- **`slide-01-opening-pptx.md`** - Full 8-slide version with extensive speaker notes
- **`slide-01-opening.html`** - HTML version with styled slides
- **`convert-to-pptx.sh`** - Conversion script (requires pandoc)

---

## 📊 Act 1 Structure (5 Slides, 8 minutes)

**Optimized for 15-minute full presentation (Acts 1-5)**

1. **Title** (0:30) - "Kubernetes AIOps Platform for Remediation"
2. **Market Landscape** (2:00) - Competitive positioning + market validation
3. **Why Automation Failed** (2:00) - Paint the pain, manual ops don't scale
4. **Why Now** (2:00) - LLM breakthrough + timing argument
5. **The Opportunity** (1:30) - Platform overview + GTM strategy
6. **References** (0:00) - Backup for Q&A

**Total Act 1 Time: 8 minutes**

See [../TIMING_GUIDE.md](../TIMING_GUIDE.md) for complete 25-minute presentation breakdown.

---

## 🎯 Quick Start: Convert to PowerPoint

### Option 1: Using Pandoc (Recommended)

**Step 1:** Install pandoc
```bash
brew install pandoc
```

**Step 2:** Run the conversion script
```bash
cd kubernaut-presentation/act1-customer-pain
./convert-to-pptx.sh
```

**Step 3:** Upload to Google Drive and open with Google Slides
- Upload `slide-01-opening.pptx` to Google Drive
- Right-click → "Open with" → "Google Slides"
- All speaker notes will be in the notes section!

---

### Option 2: Manual Conversion (No Pandoc)

**Step 1:** Open the HTML file
```bash
open slide-01-opening.html
```

**Step 2:** Print to PDF
- File → Print (or Cmd+P)
- Destination: "Save as PDF"
- Save as `slide-01-opening.pdf`

**Step 3:** Import PDF to Google Slides
- Go to Google Slides
- File → Import slides
- Select the PDF file
- Choose which slides to import

**Note:** With this method, you'll need to manually add speaker notes from the markdown file.

---

### Option 3: Manual Copy-Paste

**Step 1:** Open Google Slides and create a blank presentation

**Step 2:** Copy content from `slide-01-opening-pptx.md`
- Content between section breaks (`---`) goes on slides
- Content in `::: notes` blocks goes in speaker notes

---

## 📋 Slide Structure

The presentation contains 7 slides:

1. **Title Slide** - "The $11.2M Problem"
2. **The 3 AM Problem** - Emotional story hook
3. **The Real Cost** - Statistics and business impact
4. **What Customers Are Saying** - Social proof quote
5. **Visual Concepts** - Before/After comparison
6. **The Reality** - Urgency and key takeaway
7. **Transition** - Bridge to Act 2

---

## 🎨 Speaker Notes Format

Each slide has detailed speaker notes including:
- **Key talking points** - What to emphasize
- **Delivery tips** - How to present the content
- **Transitions** - How to move to the next slide
- **Timing guidance** - Where to pause for effect

---

## 🔧 Troubleshooting

### Pandoc not converting properly?
```bash
# Install pandoc via homebrew
brew install pandoc

# Verify installation
pandoc --version
```

### Want to customize the PowerPoint template?
1. Create a custom PowerPoint file with your branding
2. Save it as `template.pptx` in this directory
3. Run the conversion script - it will use your template

### Google Slides not preserving formatting?
- Use the PowerPoint import method for best results
- Manually adjust fonts and colors after import
- Google Slides works best with simple, clean designs

---

## 📊 Content Guidelines

### What's on the Slides (Visible to Audience)
- Clear, concise bullet points
- Key statistics and numbers
- Impactful quotes
- Simple visuals and comparisons

### What's in Speaker Notes (Only You See)
- Detailed talking points
- Stories and examples to share
- Emphasis guidance ("pause here")
- Transition cues to next slide
- Energy and delivery tips

---

## 🚀 Next Steps

After converting this slide:
1. Review speaker notes in Google Slides
2. Customize colors and fonts to match your brand
3. Add images/diagrams based on "Visual Concepts" slide
4. Practice delivery using the speaker notes
5. Create remaining slides for Act 2 and Act 3

---

## 💡 Tips for Google Slides

- **View speaker notes:** Click "View" → "Show speaker notes"
- **Present mode:** Click "Present" → Notes will show on your screen only
- **Collaboration:** Share with team for feedback
- **Version control:** File → Version history → See version history

---

## Need Help?

Check these resources:
- [Pandoc User Guide](https://pandoc.org/MANUAL.html)
- [Google Slides Help](https://support.google.com/docs/answer/2763168)
- [Markdown Presentations Guide](https://pandoc.org/MANUAL.html#producing-slide-shows-with-pandoc)

