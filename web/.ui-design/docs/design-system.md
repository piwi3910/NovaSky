# NovaSky Design System

## Overview

NovaSky uses **Mantine UI 7** with a custom theme built on **Indigo (#6366F1)** primary and **Teal (#14B8A6)** secondary colors. Dark mode is the default. All components should use design tokens — never hardcode colors, spacing, or typography values.

## Quick Start

```tsx
// main.tsx — already configured
import { novaskyTheme } from '../.ui-design/tokens/theme';
<MantineProvider theme={novaskyTheme} defaultColorScheme="dark">
```

---

## Colors

### Primary Palette (Indigo)

| Token | Hex | Usage |
|-------|-----|-------|
| primary-400 | `#818CF8` | Hover states, links |
| primary-500 | `#6366F1` | Primary actions, buttons, active nav |
| primary-600 | `#4F46E5` | Pressed/active state |
| primary-700 | `#4338CA` | Focus rings |

### Secondary Palette (Teal)

| Token | Hex | Usage |
|-------|-----|-------|
| secondary-400 | `#2DD4BF` | Accent highlights |
| secondary-500 | `#14B8A6` | Secondary actions, links |
| secondary-600 | `#0D9488` | Hover state |

### Semantic Colors

| Token | Hex | Usage |
|-------|-----|-------|
| `success` | `#22C55E` | Positive outcomes, confirmations |
| `warning` | `#F59E0B` | Caution, attention needed |
| `error` | `#EF4444` | Errors, destructive actions, UNSAFE state |
| `info` | `#6366F1` | Informational, matches primary |

### Observatory Colors

These are domain-specific colors used throughout the UI:

| Token | Hex | Usage |
|-------|-----|-------|
| `safe` | `#22C55E` | Safety state SAFE badge |
| `unsafe` | `#EF4444` | Safety state UNSAFE badge |
| `unknown` | `#F59E0B` | Safety state UNKNOWN badge |
| `day` | `#FBBF24` | Day mode badge (yellow/amber) |
| `night` | `#6366F1` | Night mode badge (indigo) |
| `twilight` | `#A78BFA` | Twilight transition |
| `starOverlay` | `rgba(0,255,100,0.6)` | Star detection circles |
| `constellationLine` | `rgba(100,150,255,0.5)` | Constellation projection lines |
| `planetLabel` | `rgba(255,200,50,0.9)` | Planet name labels |

```tsx
import { observatory } from '../.ui-design/tokens/theme';

<Badge color={state === "SAFE" ? observatory.safe : observatory.unsafe}>
```

---

## Typography

### Font Stack

- **UI text**: Inter (falls back to system fonts)
- **Data/code**: JetBrains Mono (monospace — for ADU values, coordinates, exposure times)

### Hierarchy

| Element | Component | Size | Weight | Usage |
|---------|-----------|------|--------|-------|
| Page title | `<Title order={2}>` | 1.5rem | 600 | Top of each page |
| Section header | `<Title order={5}>` or `<Text fw={500}>` | 1rem | 500 | Card headers |
| Body text | `<Text>` | 1rem | 400 | General content |
| Secondary text | `<Text size="sm" c="dimmed">` | 0.875rem | 400 | Descriptions, help text |
| Data values | `<Text size="xl" fw={700}>` | 1.25rem | 700 | Metrics, counts, readings |
| Tiny labels | `<Text size="xs" c="dimmed">` | 0.75rem | 400 | Above data values |
| Monospace data | `style={{ fontFamily: 'monospace' }}` | — | — | Coordinates, ADU, exposure |

### Pattern: Metric Card

```tsx
<div>
  <Text size="xs" c="dimmed">Stars Detected</Text>
  <Text size="xl" fw={700}>128</Text>
</div>
```

---

## Spacing

Uses Mantine's spacing scale. Always use Mantine props, never raw pixel values.

| Token | Value | Usage |
|-------|-------|-------|
| `xs` | 4px | Tight gaps (badge groups, inline items) |
| `sm` | 8px | Compact spacing (within cards) |
| `md` | 16px | Default spacing (between elements) |
| `lg` | 24px | Section spacing |
| `xl` | 32px | Page-level spacing |

```tsx
<Stack gap="md">           // 16px between items
<Group gap="xs">            // 4px between inline items
<Card padding="lg">         // 24px card padding
```

---

## Components

### Buttons

| Variant | Usage | Example |
|---------|-------|---------|
| `filled` | Primary actions | Save, Start, Calibrate |
| `light` | Secondary actions | Refresh, Add Element |
| `outline` | Tertiary actions | Calibrate North, optional actions |
| `subtle` | Inline actions | Delete (with `color="red"`) |

```tsx
<Button fullWidth>Save Settings</Button>                    // primary CTA
<Button variant="outline">Calibrate North</Button>          // optional action
<Button variant="light" color="red" size="xs">Delete</Button> // destructive
<Button variant="light" size="xs">+ Text</Button>           // add item
```

**States:**
- `loading={saving}` — show spinner during async operations
- `disabled={!isValid}` — gray out when preconditions not met

### Cards

All content sections use Mantine `<Card>` with consistent props:

```tsx
<Card shadow="sm" padding="lg" withBorder>
  <Text fw={500} mb="sm">Section Title</Text>
  {/* content */}
</Card>
```

- Always use `withBorder` for visual separation in dark mode
- `shadow="sm"` for subtle depth
- `padding="lg"` for breathing room
- Section title: `<Text fw={500} mb="sm">`

### Badges

| Color | Usage |
|-------|-------|
| `green` | SAFE, EXCELLENT, running, active, calibrated |
| `red` | UNSAFE, failed, error |
| `yellow` | UNKNOWN, POOR, warning |
| `blue` | night mode, info |
| `gray` | inactive, disabled |

```tsx
<Badge size="lg" color={stateColor} variant="filled">{state}</Badge>  // safety state
<Badge size="sm" variant="outline">{phase}</Badge>                    // auto-exposure phase
<Badge color="green">Active</Badge>                                   // status indicator
```

### Forms / Settings Pages

All Settings pages follow this pattern:

```tsx
export function SettingsExample() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [field, setField] = useState(defaultValue);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      // populate state from config
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/key", { method: "PUT", ... });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Page Title</Title>
      <Card shadow="sm" padding="lg" withBorder>
        {/* form fields */}
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save</Button>
    </Stack>
  );
}
```

**Form fields:**
- Use `<NumberInput>` for numeric values with `min`/`max`/`step`
- Use `<Select>` for enum choices
- Use `<Switch>` for boolean toggles with `description` prop
- Use `<TextInput>` for text values
- Group related fields with `<Group grow>`
- Show conditional fields only when their toggle is on

### Switches

```tsx
<Switch
  label="Enable stacking"
  checked={enabled}
  onChange={e => setEnabled(e.currentTarget.checked)}
  description="Stack multiple frames before processing"
  mb="sm"
/>
```

### Charts (Recharts)

Use these colors for chart consistency:

```tsx
<Line stroke="#6366F1" />   // primary data series (indigo)
<Line stroke="#14B8A6" />   // secondary series (teal)
<Line stroke="#F59E0B" />   // warning/highlight series (amber)
<Line stroke="#94A3B8" />   // neutral/reference series (slate)
```

- Background: transparent (inherits card background)
- Grid: `stroke="#334155"` (slate-700)
- Axis text: `fill="#94A3B8"` (slate-400)
- Tooltip: dark background with light text

### Detection Overlays (Canvas)

When drawing on the canvas overlay:

```tsx
// Stars — green circles
ctx.strokeStyle = "rgba(0, 255, 100, 0.6)";
ctx.lineWidth = 1;

// Constellations — blue lines + labels
ctx.strokeStyle = "rgba(100, 150, 255, 0.5)";
ctx.fillStyle = "rgba(100, 150, 255, 0.8)";
ctx.lineWidth = 1.5;

// Planets — yellow dots + labels
ctx.fillStyle = "rgba(255, 200, 50, 0.9)";

// Mask circle — green outline
ctx.strokeStyle = "#00ff00";      // enabled
ctx.strokeStyle = "rgba(0,255,0,0.5)"; // disabled (dashed)

// Center crosshair — red
ctx.strokeStyle = "rgba(255,0,0,0.7)";
```

### Navigation (AppShell)

```tsx
<AppShell navbar={{ width: 220 }}>
  <AppShell.Navbar>
    <ScrollArea>
      <NavLink label="Dashboard" />           // top-level page
      <NavLink label="Settings" defaultOpened> // collapsible group
        <NavLink label="Camera" />             // nested setting
      </NavLink>
    </ScrollArea>
  </AppShell.Navbar>
</AppShell>
```

- Navbar width: 220px
- Wrap in `<ScrollArea>` for overflow
- Use `defaultOpened` on Settings group

---

## Dark Mode

Dark mode is the default (`defaultColorScheme="dark"`). Key considerations:

- **Card borders**: Always use `withBorder` — essential for visibility in dark mode
- **Dimmed text**: Use `c="dimmed"` instead of hardcoded grays
- **Backgrounds**: Let Mantine handle — don't set explicit dark backgrounds on components
- **Canvas/overlays**: Use `background: "#000"` for image containers
- **Inverted text**: For text on colored badges, Mantine handles contrast automatically

---

## File Structure

```
web/.ui-design/
  design-system.json     # Master token configuration
  tokens/
    theme.ts             # Mantine createTheme() — import in main.tsx
    tokens.css           # CSS custom properties (for canvas/non-Mantine use)
  docs/
    design-system.md     # This file
```

## Updating

To apply the theme, update `web/src/main.tsx`:

```tsx
import { novaskyTheme } from '../.ui-design/tokens/theme';

<MantineProvider theme={novaskyTheme} defaultColorScheme="dark">
```
