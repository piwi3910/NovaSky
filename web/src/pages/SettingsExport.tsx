import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, TextInput, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsExport() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [exportDir, setExportDir] = useState("/home/piwi/novasky-data/export");
  const [saveFits, setSaveFits] = useState(true);
  const [saveJpeg, setSaveJpeg] = useState(true);
  const [saveOverlay, setSaveOverlay] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const exp = (configData["export"] ?? {}) as any;
      setExportDir(exp.dir ?? "/home/piwi/novasky-data/export");
      setSaveFits(exp.saveFits ?? true);
      setSaveJpeg(exp.saveJpeg ?? true);
      setSaveOverlay(exp.saveOverlay ?? false);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    try {
      await fetch("/api/config/export", {
        method: "PUT", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value: { dir: exportDir, saveFits, saveJpeg, saveOverlay } }),
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Stack gap="md">
      <Title order={2}>Export Settings</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <TextInput label="Export Directory" value={exportDir} onChange={e => setExportDir(e.currentTarget.value)} mb="md" />
        <Switch label="Save FITS files" checked={saveFits} onChange={e => setSaveFits(e.currentTarget.checked)} mb="sm" />
        <Switch label="Save JPEG previews" checked={saveJpeg} onChange={e => setSaveJpeg(e.currentTarget.checked)} mb="sm" />
        <Switch label="Save overlay-burned copies" checked={saveOverlay} onChange={e => setSaveOverlay(e.currentTarget.checked)} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth color={saved ? "green" : undefined}>{saved ? "Saved!" : "Save Export Settings"}</Button>
    </Stack>
  );
}
