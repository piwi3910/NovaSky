import { useState } from "react";
import { Stack, Title, Table, Pagination, Group, Text, Loader } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { formatDate } from "../utils/format";

interface FramesResponse { frames: Array<{ id: string; capturedAt: string; exposureMs: number; gain: number; filePath: string }>; }

export function Frames() {
  const [page, setPage] = useState(1);
  const limit = 20;
  const { data, loading } = useApi<FramesResponse>(`/api/frames?limit=${limit}&offset=${(page - 1) * limit}`, 10000);

  return (
    <Stack gap="md">
      <Title order={2}>Captured Frames</Title>
      <Table striped highlightOnHover>
        <Table.Thead><Table.Tr><Table.Th>Time</Table.Th><Table.Th>Exposure</Table.Th><Table.Th>Gain</Table.Th><Table.Th>File</Table.Th></Table.Tr></Table.Thead>
        <Table.Tbody>
          {data?.frames?.map((f) => (
            <Table.Tr key={f.id}>
              <Table.Td>{formatDate(f.capturedAt)}</Table.Td>
              <Table.Td>{f.exposureMs.toFixed(3)} ms</Table.Td>
              <Table.Td>{f.gain}</Table.Td>
              <Table.Td><Text size="xs" c="dimmed">{f.filePath.split("/").pop()}</Text></Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      {loading && <Group justify="center" py="xl"><Loader /></Group>}
      {(!data?.frames || data.frames.length === 0) && !loading && (
        <Text c="dimmed" ta="center" py="xl">No frames captured yet</Text>
      )}
      <Group justify="center"><Pagination value={page} onChange={setPage} total={Math.max(page, (data?.frames?.length ?? 0) === limit ? page + 1 : page)} /></Group>
    </Stack>
  );
}
