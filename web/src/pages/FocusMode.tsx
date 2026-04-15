import { useState } from "react";
import { Stack, Title, Card, Text, Button, Group, Badge } from "@mantine/core";
import { useApi } from "../hooks/useApi";

interface FocusStatus { focusMode: boolean; }

export function FocusMode() {
  const { data } = useApi<FocusStatus>("/api/focus/status", 2000);
  const [loading, setLoading] = useState(false);

  async function toggle() {
    setLoading(true);
    const endpoint = data?.focusMode ? "/api/focus/stop" : "/api/focus/start";
    await fetch(endpoint, { method: "POST" });
    setLoading(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Focus Mode</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Group justify="space-between" mb="md">
          <Text fw={500}>Status</Text>
          <Badge size="lg" color={data?.focusMode ? "green" : "gray"} variant="filled">
            {data?.focusMode ? "ACTIVE" : "INACTIVE"}
          </Badge>
        </Group>
        <Text size="sm" c="dimmed" mb="md">
          Focus mode captures rapid frames with fixed exposure for focusing the camera lens.
          Normal auto-exposure capture pauses while focus mode is active.
        </Text>
        <Button onClick={toggle} loading={loading} fullWidth color={data?.focusMode ? "red" : "green"}>
          {data?.focusMode ? "Stop Focus Mode" : "Start Focus Mode"}
        </Button>
      </Card>
    </Stack>
  );
}
