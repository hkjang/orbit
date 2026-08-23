export interface User {
  id: string;
  username: string;
  email: string;
  display_name: string;
  role: "admin" | "team_lead" | "member";
  status: "active" | "disabled";
  last_login_at?: string;
  created_at: string;
}

export interface PublicConfig {
  service_name: string;
  version: string;
  commit: string;
  built_at: string;
  oidc: { enabled: boolean; display_name: string };
}

export interface Person {
  id: string;
  display_name: string;
  company: string;
  role_title: string;
  avatar_url: string;
  email: string;
  phone: string;
  note: string;
  first_met?: string;
  importance: number;
  closeness: number;
  momentum: number;
  stable_x: number;
  stable_y: number;
  categories: string[];
  relationship_label: string;
  last_interaction_at?: string;
  /** 고정된 관계. 교류가 줄어도 바깥 궤도로 밀려나지 않습니다. */
  anchored?: boolean;
  created_at: string;
}

export interface OrbitNode {
  id: string;
  name: string;
  avatar_url: string;
  importance: number;
  closeness: number;
  momentum: number;
  x: number;
  y: number;
  categories: string[];
  label: string;
  last_interaction_at?: string;
  /** 이 사람과 묶인 승인된 기억 수. Memory Nebula의 크기를 정합니다. */
  memory_count?: number;
  anchored?: boolean;
}

export interface Memory {
  id: string;
  person_id?: string;
  person_name?: string;
  title: string;
  content: string;
  occurred_at?: string;
  source_type: string;
  source_reference: string;
  topics: string[];
  status: "draft" | "pending" | "approved" | "rejected";
  created_at: string;
}

export interface Interaction {
  id: string;
  kind: string;
  occurred_at: string;
  weight: number;
  summary: string;
  source: string;
  created_at: string;
}
