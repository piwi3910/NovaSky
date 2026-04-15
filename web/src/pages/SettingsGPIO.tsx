import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, NumberInput, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsGPIO() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [i2cEnabled, setI2cEnabled] = useState(false);
  const [rainPin, setRainPin] = useState(0);
  const [dewEnabled, setDewEnabled] = useState(false);
  const [dewPin, setDewPin] = useState(0);
  const [dewDelta, setDewDelta] = useState(3.0);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const gpio = (configData["gpio"] ?? {}) as any;
      setI2cEnabled(gpio.i2cEnabled ?? false);
      setRainPin(gpio.rainPin ?? 0);
      setDewEnabled(gpio.dewHeater?.enabled ?? false);
      setDewPin(gpio.dewHeater?.pin ?? 0);
      setDewDelta(gpio.dewHeater?.deltaTemp ?? 3.0);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/gpio", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { i2cEnabled, rainPin, dewHeater: { enabled: dewEnabled, pin: dewPin, deltaTemp: dewDelta } } }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>GPIO / Sensors</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable I2C sensors (BME280/680)" checked={i2cEnabled} onChange={e => setI2cEnabled(e.currentTarget.checked)} mb="md" />
        <NumberInput label="Rain sensor GPIO pin" value={rainPin} onChange={v => setRainPin(Number(v))} min={0} max={27} />
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Dew Heater</Text>
        <Switch label="Enable dew heater" checked={dewEnabled} onChange={e => setDewEnabled(e.currentTarget.checked)} mb="sm" />
        <NumberInput label="PWM GPIO pin" value={dewPin} onChange={v => setDewPin(Number(v))} min={0} max={27} disabled={!dewEnabled} mb="sm" />
        <NumberInput label="Target delta above dew point (°C)" value={dewDelta} onChange={v => setDewDelta(Number(v))} min={1} max={10} decimalScale={1} disabled={!dewEnabled} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save GPIO Settings</Button>
    </Stack>
  );
}
