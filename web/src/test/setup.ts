import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// vitest의 globals가 꺼져 있으면 Testing Library가 정리를 스스로 걸지 못한다.
// 그러면 앞선 테스트가 그린 DOM이 남아, 같은 글자를 찾는 뒷 테스트가 엉뚱한
// 요소를 집거나 "여러 개가 걸렸다"며 실패한다. 여기서 한 번 걸어 둔다.
afterEach(cleanup);
