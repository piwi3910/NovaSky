import { useState } from "react";
import { Stack, Title, Card, Text, Select, Slider, Group, Button, SegmentedControl } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function ProcessingTuner() {
  const { data: status } = useApi<any>("/api/status", 5000);
  const [stretch, setStretch] = useState("none");
  const [scnrEnabled, setScnrEnabled] = useState(true);
  const [noiseFilter, setNoiseFilter] = useState("off");
  const [noiseKernel, setNoiseKernel] = useState(3);
  const [skyglowAggr, setSkyglowAggr] = useState(0);
  const [previewUrl, setPreviewUrl] = useState("");
  const [processing, setProcessing] = useState(false);

  const frameId = status?.frame?.id;

  async function preview() {
    if (!frameId) return;
    setProcessing(true);
    const params = new URLSearchParams({
      frameId, stretch, scnr: scnrEnabled ? "on" : "off",
      noiseFilter, noiseKernel: String(noiseKernel),
      skyglowAggressiveness: String(skyglowAggr),
    });
    setPreviewUrl(`/api/process-preview?${params}&t=${Date.now()}`);
    setProcessing(false);
  }

  async function saveToProfile(profile: string) {
    await fetch(`/api/config/imaging.${profile}`, {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { stretch, scnr: scnrEnabled, noiseFilter, noiseKernel, skyglowAggressiveness: skyglowAggr } }),
    });
  }

  return (
    <Stack gap="md">
      <Title order={2}>Image Processing Tuner</Title>

      <Card shadow="sm" padding="lg" withBorder>
        <Select label="Stretch Mode" data={[
          { value: "none", label: "None (linear 16→8)" },
          { value: "linear", label: "Linear (percentile)" },
          { value: "auto", label: "Auto (per-channel)" },
          { value: "adaptive", label: "Adaptive (MTF)" },
          { value: "ghs", label: "GHS (Arcsinh)" },
        ]} value={stretch} onChange={v => setStretch(v ?? "none")} mb="sm" />

        <SegmentedControl data={[{ value: "on", label: "SCNR On" }, { value: "off", label: "SCNR Off" }]}
          value={scnrEnabled ? "on" : "off"} onChange={v => setScnrEnabled(v === "on")} mb="sm" fullWidth />

        <Select label="Noise Filter" data={[
          { value: "off", label: "Off" }, { value: "gaussian", label: "Gaussian" },
          { value: "bilateral", label: "Bilateral" }, { value: "median", label: "Median" },
        ]} value={noiseFilter} onChange={v => setNoiseFilter(v ?? "off")} mb="sm" />

        <Text size="sm" mb={4}>Noise Kernel Size: {noiseKernel}</Text>
        <Slider value={noiseKernel} onChange={setNoiseKernel} min={3} max={15} step={2} mb="sm" />

        <Text size="sm" mb={4}>Skyglow Reduction: {skyglowAggr === 0 ? "Off" : skyglowAggr}</Text>
        <Slider value={skyglowAggr} onChange={setSkyglowAggr} min={0} max={128} step={8} mb="md" />

        <Group>
          <Button onClick={preview} loading={processing}>Preview</Button>
          <Button onClick={() => saveToProfile("day")} variant="outline">Save to Day Profile</Button>
          <Button onClick={() => saveToProfile("night")} variant="outline">Save to Night Profile</Button>
        </Group>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Preview</Text>
        {previewUrl ? (
          <img src={previewUrl} alt="Processing preview"
            style={{ width: "100%", maxHeight: 600, objectFit: "contain", background: "#000", borderRadius: 8 }} />
        ) : (
          <Text c="dimmed" ta="center" py="xl">Click Preview to see processing result</Text>
        )}
      </Card>
    </Stack>
  );
}
