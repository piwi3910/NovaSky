import { Stack, Title, Table, Badge } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { formatDate } from "../utils/format";

interface HistoryResponse { history: Array<{ id: string; state: string; imagingQuality: string; reason: string | null; evaluatedAt: string }>; }
const STATE_COLORS: Record<string, string> = { SAFE: "green", UNSAFE: "red", UNKNOWN: "yellow" };

export function History() {
  const { data } = useApi<HistoryResponse>("/api/safety-history?limit=100", 10000);
  return (
    <Stack gap="md">
      <Title order={2}>Safety History</Title>
      <Table striped highlightOnHover>
        <Table.Thead><Table.Tr><Table.Th>Time</Table.Th><Table.Th>State</Table.Th><Table.Th>Quality</Table.Th><Table.Th>Reason</Table.Th></Table.Tr></Table.Thead>
        <Table.Tbody>
          {data?.history?.map((e) => (
            <Table.Tr key={e.id}>
              <Table.Td>{formatDate(e.evaluatedAt)}</Table.Td>
              <Table.Td><Badge color={STATE_COLORS[e.state] ?? "gray"} variant="filled">{e.state}</Badge></Table.Td>
              <Table.Td><Badge variant="outline">{e.imagingQuality}</Badge></Table.Td>
              <Table.Td>{e.reason ?? "—"}</Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Stack>
  );
}
