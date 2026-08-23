import { useEffect, useMemo, useState } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Typography,
} from "@mui/material";
import RocketLaunchRoundedIcon from "@mui/icons-material/RocketLaunchRounded";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import { findApproach } from "../gravityAssist";
import { readGrammar } from "../orbitGrammar";
import type { OrbitLink, OrbitNode, Person } from "../types";

/**
 * 소원해진 사람에게 다시 닿는 길을 제안합니다.
 * 가까운 사람에게는 굳이 우회로를 권할 이유가 없으므로 카드 자체가 나타나지 않습니다.
 */
export function GravityAssistCard({ person }: { person: Person }) {
  const navigate = useNavigate();
  const [orbit, setOrbit] = useState<{
    nodes: OrbitNode[];
    links: OrbitLink[];
  }>();
  const grammar = readGrammar(person);
  const distant = grammar.state === "drifting" || grammar.state === "dormant";
  useEffect(() => {
    if (!distant) return;
    api<{ nodes: OrbitNode[]; links: OrbitLink[] }>("/orbit")
      .then((data) => setOrbit({ nodes: data.nodes, links: data.links ?? [] }))
      .catch(() => setOrbit({ nodes: [], links: [] }));
  }, [distant]);
  const route = useMemo(
    () =>
      orbit ? findApproach(person.id, orbit.nodes, orbit.links) : undefined,
    [orbit, person.id],
  );
  if (!distant || !route) return null;
  const bridges = route.steps.slice(0, -1);
  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <RocketLaunchRoundedIcon color="secondary" fontSize="small" />
          <Typography variant="h3">다시 닿는 길</Typography>
        </Box>
        {route.direct ? (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5 }}>
            지금은 직접 연락하는 것이 가장 자연스러운 길입니다. 함께 아는 사람을
            이어두면 다른 길도 보이게 됩니다.
          </Typography>
        ) : (
          <>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5 }}>
              {bridges.map((b) => b.name).join(", ")}님을 거치면 더 자연스럽게
              닿을 수 있습니다.
            </Typography>
            <Box sx={{ mt: 2 }}>
              <Step label="나" first />
              {route.steps.map((step, index) => (
                <Step
                  key={step.person_id}
                  label={step.name}
                  target={index === route.steps.length - 1}
                  onClick={
                    index === route.steps.length - 1
                      ? undefined
                      : () => navigate(`/people/${step.person_id}`)
                  }
                />
              ))}
            </Box>
            <Button
              size="small"
              sx={{ mt: 1.5 }}
              onClick={() => navigate(`/ai?person=${bridges[0].person_id}`)}
            >
              {bridges[0].name}님에게 어떻게 말을 꺼낼지 AI에게 묻기
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function Step({
  label,
  target,
  first,
  onClick,
}: {
  label: string;
  target?: boolean;
  first?: boolean;
  onClick?: () => void;
}) {
  return (
    <Box>
      {!first && (
        <Box sx={{ width: 2, height: 14, bgcolor: "divider", ml: 1.6 }} />
      )}
      <Chip
        size="small"
        label={label}
        color={target ? "secondary" : "default"}
        variant={target ? "filled" : "outlined"}
        clickable={Boolean(onClick)}
        onClick={onClick}
      />
    </Box>
  );
}
