import { Stack, Title, Card, Text, Group, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

interface TimelapseResponse {
  timelapses: Array<{ name: string; path: string; size: number }>;
}

export function Timelapse() {
  const { data } = useApi<TimelapseResponse>("/api/timelapse", 30000);

  return (
    <Stack gap="md">
      <Title order={2}>Timelapses</Title>
      {data?.timelapses?.length ? (
        data.timelapses.map((tl) => (
          <Card key={tl.name} shadow="sm" padding="lg" withBorder>
            <Group justify="space-between">
              <div>
                <Text fw={500}>{tl.name}</Text>
                <Text size="sm" c="dimmed">{(tl.size / 1024 / 1024).toFixed(1)} MB</Text>
              </div>
              <Button component="a" href={`/api/timelapse/${tl.name}`} target="_blank" variant="outline">
                Download
              </Button>
            </Group>
          </Card>
        ))
      ) : (
        <Text c="dimmed">No timelapses generated yet. They are created automatically every 100 frames or hourly.</Text>
      )}
    </Stack>
  );
}
