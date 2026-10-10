import {Frame, Screen} from './Frame';
export const Models = () => <Frame>
 <div style={{position:'absolute',left:80,top:166,fontSize:84,fontWeight:700,letterSpacing:-3}}>Choose your models.</div>
 <div style={{position:'absolute',left:84,top:273,fontSize:44,color:'#c9d5cc',letterSpacing:-1}}>Optional. Use your saved defaults.</div>
 <Screen src="models.jpg" top={360} height={600} />
</Frame>;
