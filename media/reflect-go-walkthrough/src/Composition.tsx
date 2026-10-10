import {Composition, Folder, Series, Still, useVideoConfig} from 'remotion';
import {Opening} from './Opening';
import {OpenApp} from './OpenApp';
import {Models} from './Models';
import {Project} from './Project';
import {Client} from './Client';
import {Closing} from './Closing';

export const Walkthrough = () => {
 const {fps}=useVideoConfig();
 return <Series>
  <Series.Sequence name="Your AI on your drive" durationInFrames={3*fps} premountFor={fps}><Opening /></Series.Sequence>
  <Series.Sequence name="Open Reflect Go" durationInFrames={5*fps} premountFor={fps}><OpenApp /></Series.Sequence>
  <Series.Sequence name="Choose models if you want" durationInFrames={4*fps} premountFor={fps}><Models /></Series.Sequence>
  <Series.Sequence name="Choose project if you want" durationInFrames={4*fps} premountFor={fps}><Project /></Series.Sequence>
  <Series.Sequence name="Open your client" durationInFrames={4*fps} premountFor={fps}><Client /></Series.Sequence>
  <Series.Sequence name="Bring your weights" durationInFrames={4*fps} premountFor={fps}><Closing /></Series.Sequence>
 </Series>;
};
export const Poster = () => <Opening still />;
export const MyComposition = () => <>
 <Composition id="ReflectGoWalkthrough" component={Walkthrough} durationInFrames={720} fps={30} width={1080} height={1080} />
 <Still id="ReflectGoPoster" component={Poster} width={1080} height={1080} />
 <Folder name="Scenes">
  <Composition id="Opening" component={Opening} durationInFrames={90} fps={30} width={1080} height={1080} />
  <Composition id="OpenApp" component={OpenApp} durationInFrames={150} fps={30} width={1080} height={1080} />
  <Composition id="Models" component={Models} durationInFrames={120} fps={30} width={1080} height={1080} />
  <Composition id="Project" component={Project} durationInFrames={120} fps={30} width={1080} height={1080} />
  <Composition id="Client" component={Client} durationInFrames={120} fps={30} width={1080} height={1080} />
  <Composition id="Closing" component={Closing} durationInFrames={120} fps={30} width={1080} height={1080} />
 </Folder>
</>;
