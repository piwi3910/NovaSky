import { useState, useEffect } from "react";
import {
  Card,
  Title,
  Stack,
  Group,
  Button,
  TextInput,
  Switch,
  Text,
  Select,
  NumberInput,
  ActionIcon,
  Table,
  Badge,
  Divider,
  ColorInput,
} from "@mantine/core";

interface OverlayElement {
  type: "text" | "variable" | "compass" | "grid" | "crosshair";
  x: number;
  y: number;
  content?: string;
  fontSize?: number;
  color?: string;
  enabled: boolean;
}

interface OverlayLayout {
  id: string;
  name: string;
  layout: OverlayElement[];
  isActive: boolean;
  createdAt: string;
}

const VARIABLES = [
  "{date}", "{time}", "{exposure}", "{gain}", "{adu}",
  "{temp}", "{humidity}", "{sqm}", "{bortle}", "{moon}", "{cloudcover}",
];

export function OverlayEditor() {
  const [layouts, setLayouts] = useState<OverlayLayout[]>([]);
  const [layerConfig, setLayerConfig] = useState<Record<string, boolean>>({});
  const [selected, setSelected] = useState<OverlayLayout | null>(null);
  const [elements, setElements] = useState<OverlayElement[]>([]);
  const [newName, setNewName] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadLayouts();
    loadLayerConfig();
  }, []);

  const loadLayouts = async () => {
    try {
      const res = await fetch("/api/overlay/layouts");
      const data = await res.json();
      setLayouts(data ?? []);
    } catch { /* empty */ }
  };

  const loadLayerConfig = async () => {
    try {
      const res = await fetch("/api/overlay/config");
      const data = await res.json();
      setLayerConfig(data);
    } catch { /* empty */ }
  };

  const toggleLayer = async (key: string) => {
    const updated = { ...layerConfig, [key]: !layerConfig[key] };
    setLayerConfig(updated);
    try {
      await fetch("/api/overlay/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(updated),
      });
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    }
  };

  const createLayout = async () => {
    if (!newName.trim()) return;
    try {
      const res = await fetch("/api/overlay/layouts", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: newName, layout: [] }),
      });
      const layout = await res.json();
      setLayouts([layout, ...layouts]);
      setNewName("");
      selectLayout(layout);
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    }
  };

  const selectLayout = (layout: OverlayLayout) => {
    setSelected(layout);
    setElements(Array.isArray(layout.layout) ? layout.layout : []);
  };

  const addElement = (type: OverlayElement["type"]) => {
    const el: OverlayElement = {
      type,
      x: 10,
      y: 10 + elements.length * 30,
      content: type === "variable" ? "{date}" : type === "text" ? "Custom Text" : undefined,
      fontSize: 14,
      color: "#ffffff",
      enabled: true,
    };
    setElements([...elements, el]);
  };

  const updateElement = (idx: number, updates: Partial<OverlayElement>) => {
    const updated = [...elements];
    updated[idx] = { ...updated[idx], ...updates };
    setElements(updated);
  };

  const removeElement = (idx: number) => {
    setElements(elements.filter((_, i) => i !== idx));
  };

  const saveLayout = async () => {
    if (!selected) return;
    setSaving(true);
    try {
      await fetch(`/api/overlay/layouts/${selected.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: selected.name, layout: elements }),
      });
      await loadLayouts();
    } finally {
      setSaving(false);
    }
  };

  const activateLayout = async (id: string) => {
    try {
      await fetch(`/api/overlay/layouts/${id}/activate`, { method: "PUT" });
      await loadLayouts();
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    }
  };

  const deleteLayout = async (id: string) => {
    try {
      await fetch(`/api/overlay/layouts/${id}`, { method: "DELETE" });
      if (selected?.id === id) {
        setSelected(null);
        setElements([]);
      }
      await loadLayouts();
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    }
  };

  return (
    <Stack gap="md">
      <Title order={2}>Overlay Editor</Title>

      <Card shadow="sm" padding="lg" withBorder>
        <Title order={5} mb="sm">Layer Visibility</Title>
        <Group>
          {Object.entries(layerConfig).map(([key, enabled]) => (
            <Switch
              key={key}
              label={key.replace(/([A-Z])/g, " $1").trim()}
              checked={enabled}
              onChange={() => toggleLayer(key)}
            />
          ))}
        </Group>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Title order={5} mb="sm">Overlay Layouts</Title>
        <Group mb="md">
          <TextInput
            placeholder="Layout name"
            value={newName}
            onChange={(e) => setNewName(e.currentTarget.value)}
            style={{ flex: 1 }}
          />
          <Button onClick={createLayout} disabled={!newName.trim()}>Create</Button>
        </Group>

        {layouts.length > 0 && (
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Actions</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {layouts.map((l) => (
                <Table.Tr
                  key={l.id}
                  style={{ cursor: "pointer", background: selected?.id === l.id ? "var(--mantine-color-dark-5)" : undefined }}
                  onClick={() => selectLayout(l)}
                >
                  <Table.Td>{l.name}</Table.Td>
                  <Table.Td>
                    {l.isActive
                      ? <Badge color="green">Active</Badge>
                      : <Badge color="gray">Inactive</Badge>}
                  </Table.Td>
                  <Table.Td>
                    <Group gap="xs">
                      <Button size="xs" variant="light" onClick={(e) => { e.stopPropagation(); activateLayout(l.id); }} disabled={l.isActive}>Activate</Button>
                      <Button size="xs" variant="light" color="red" onClick={(e) => { e.stopPropagation(); deleteLayout(l.id); }}>Delete</Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      </Card>

      {selected && (
        <Card shadow="sm" padding="lg" withBorder>
          <Group justify="space-between" mb="md">
            <Title order={5}>Edit: {selected.name}</Title>
            <Button onClick={saveLayout} loading={saving}>Save Layout</Button>
          </Group>

          <Group mb="md">
            <Button size="xs" variant="light" onClick={() => addElement("text")}>+ Text</Button>
            <Button size="xs" variant="light" onClick={() => addElement("variable")}>+ Variable</Button>
            <Button size="xs" variant="light" onClick={() => addElement("compass")}>+ Compass</Button>
            <Button size="xs" variant="light" onClick={() => addElement("grid")}>+ Grid</Button>
            <Button size="xs" variant="light" onClick={() => addElement("crosshair")}>+ Crosshair</Button>
          </Group>

          <Divider mb="md" />

          <Stack gap="sm">
            {elements.map((el, idx) => (
              <Card key={idx} withBorder p="xs">
                <Group justify="space-between" align="flex-start">
                  <Group align="flex-end" gap="sm" wrap="wrap">
                    <Badge size="sm" variant="outline">{el.type}</Badge>
                    <Switch label="On" size="xs" checked={el.enabled} onChange={(e) => updateElement(idx, { enabled: e.currentTarget.checked })} />
                    {(el.type === "text" || el.type === "variable") && (
                      <>
                        {el.type === "variable" ? (
                          <Select size="xs" label="Variable" data={VARIABLES} value={el.content} onChange={(v) => updateElement(idx, { content: v || "{date}" })} w={140} />
                        ) : (
                          <TextInput size="xs" label="Text" value={el.content} onChange={(e) => updateElement(idx, { content: e.currentTarget.value })} w={200} />
                        )}
                        <NumberInput size="xs" label="Size" value={el.fontSize} onChange={(v) => updateElement(idx, { fontSize: Number(v) || 14 })} w={70} min={8} max={72} />
                        <ColorInput size="xs" label="Color" value={el.color} onChange={(v) => updateElement(idx, { color: v })} w={120} />
                      </>
                    )}
                    <NumberInput size="xs" label="X" value={el.x} onChange={(v) => updateElement(idx, { x: Number(v) || 0 })} w={70} />
                    <NumberInput size="xs" label="Y" value={el.y} onChange={(v) => updateElement(idx, { y: Number(v) || 0 })} w={70} />
                  </Group>
                  <ActionIcon color="red" variant="subtle" onClick={() => removeElement(idx)}>X</ActionIcon>
                </Group>
              </Card>
            ))}
            {elements.length === 0 && (
              <Text c="dimmed" ta="center">No elements yet. Add text, variables, or overlays above.</Text>
            )}
          </Stack>
        </Card>
      )}
    </Stack>
  );
}
